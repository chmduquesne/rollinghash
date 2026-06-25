package rollinghash

import (
	"hash"
	"io"
)

// chunkerBatchSize is the read/hash batch the Chunker uses. Kept modest so the
// per-batch work stays cache-resident.
const chunkerBatchSize = 16 << 10

// A Chunker splits an io.Reader into content-defined chunks. A boundary is
// placed after the first byte at which the rolling checksum (over the preceding
// window bytes) satisfies checksum & mask == 0, subject to a chunk length in
// [min, max]; if no such boundary is found by max, the chunk is cut at max. The
// trailing bytes of the stream form a final chunk.
//
//	c := NewChunker(r, h, window, mask, min, max)
//	for c.Next() {
//		chunk := c.Chunk()
//		if c.AtMask() {
//			// content-defined boundary; c.Sum() is the hit value
//		} else {
//			// forced cut at max, or the final chunk at end of stream
//		}
//	}
//	if err := c.Err(); err != nil { ... }
//
// The boundary search is fused into the hashing loop via BoundaryRoller when
// the hash implements it (no checksum stream is materialized); otherwise it
// falls back to BulkRoll, or to Write+Roll. A stream shorter than window yields
// no chunks.
type Chunker struct {
	r      io.Reader
	h      Hash
	brd    BoundaryRoller // fast path; nil -> fallback
	bulk   BulkRoller     // fallback path; nil -> Roll fallback
	sum    func() uint64  // reads h's current sum, for Sum() recompute
	window int
	mask   uint64
	min    int
	max    int

	// read/hash buffer, carrying window-1 bytes between batches like Scanner
	rbuf       []byte
	carry      int
	prevN      int
	firstBatch bool
	eof        bool

	la, lb []int32  // fused lane hit buffers
	scan   []uint64 // BulkRoll fallback scratch

	// chunk byte accumulator; cbuf[head] is the byte at global offset chunkStart
	cbuf       []byte
	head       int
	chunkStart int
	consumed   int // global offset of the next not-yet-buffered byte

	bounds []int // ascending global boundary-byte positions, not yet consumed
	bcur   int

	done   bool
	err    error
	chunk  []byte
	sumv   uint64
	atMask bool
}

// NewChunker returns a Chunker over r. A boundary is placed where the rolling
// checksum under h (over window bytes) satisfies checksum & mask == 0, with the
// chunk length kept in [min, max]. window must be >= 1 and window <= min <= max
// for well-formed output.
func NewChunker(r io.Reader, h Hash, window int, mask uint64, min, max int) *Chunker {
	bufSize := chunkerBatchSize
	if bufSize < window {
		bufSize = window
	}
	maxOut := bufSize - window + 1
	if maxOut < 1 {
		maxOut = 1
	}
	brd, _ := h.(BoundaryRoller)
	bulk, _ := h.(BulkRoller)
	c := &Chunker{
		r:          r,
		h:          h,
		brd:        brd,
		bulk:       bulk,
		window:     window,
		mask:       mask,
		min:        min,
		max:        max,
		rbuf:       make([]byte, bufSize),
		firstBatch: true,
		la:         make([]int32, maxOut),
		lb:         make([]int32, maxOut),
	}
	if brd == nil && bulk != nil {
		c.scan = make([]uint64, maxOut)
	}
	switch v := h.(type) {
	case hash.Hash64:
		c.sum = v.Sum64
	case hash.Hash32:
		c.sum = func() uint64 { return uint64(v.Sum32()) }
	default:
		var b [8]byte
		c.sum = func() uint64 {
			var r uint64
			for _, x := range h.Sum(b[:0]) {
				r = r<<8 | uint64(x)
			}
			return r
		}
	}
	return c
}

// Reset prepares the Chunker to split r from the start, reusing its buffers.
func (c *Chunker) Reset(r io.Reader) {
	c.r = r
	c.carry = 0
	c.prevN = 0
	c.firstBatch = true
	c.eof = false
	c.cbuf = c.cbuf[:0]
	c.head = 0
	c.chunkStart = 0
	c.consumed = 0
	c.bounds = c.bounds[:0]
	c.bcur = 0
	c.done = false
	c.err = nil
	c.chunk = nil
	c.sumv = 0
	c.atMask = false
}

// Next advances to the next chunk, returning false at end of input or on the
// first error. After it returns false, Err reports any error other than EOF.
func (c *Chunker) Next() bool {
	if c.err != nil || c.done {
		c.chunk = nil
		c.sumv = 0
		c.atMask = false
		return false
	}
	for {
		minByte := c.chunkStart + c.min - 1 // smallest boundary byte with L >= min
		maxByte := c.chunkStart + c.max - 1 // forced-cut boundary byte (L == max)

		// First mask boundary with min <= L <= max, among the boundaries known
		// so far.
		for c.bcur < len(c.bounds) {
			e := c.bounds[c.bcur]
			if e < minByte {
				c.bcur++ // too short for this chunk; never reusable
				continue
			}
			if e <= maxByte {
				c.bcur++
				return c.emit(e, true)
			}
			break // next boundary is past max; force a cut instead
		}

		// No in-range mask boundary: force a cut at max once those bytes exist.
		if c.consumed-1 >= maxByte {
			return c.emit(maxByte, false)
		}

		// Need more data.
		if !c.readBatch() {
			if c.err != nil {
				return c.fail()
			}
			if c.head < len(c.cbuf) { // trailing bytes -> final chunk
				c.done = true
				return c.emit(c.consumed-1, false)
			}
			return c.fail()
		}
	}
}

// emit records the chunk ending at global byte e and advances past it.
func (c *Chunker) emit(e int, atMask bool) bool {
	l := e - c.chunkStart + 1
	c.chunk = c.cbuf[c.head : c.head+l]
	c.sumv = c.windowSum(e)
	c.atMask = atMask
	c.head += l
	c.chunkStart += l
	return true
}

// windowSum recomputes the rolling checksum of the window ending at global byte
// e from the buffered bytes (cheap: once per emitted chunk). Returns 0 when the
// window is not fully buffered (a final chunk shorter than window).
func (c *Chunker) windowSum(e int) uint64 {
	start := e - c.window + 1
	if start < c.chunkStart {
		return 0
	}
	off := c.head + (start - c.chunkStart)
	c.h.Reset()
	c.h.Write(c.cbuf[off : off+c.window])
	return c.sum()
}

// readBatch reads the next block (carrying window-1 bytes for continuity),
// appends its new bytes to cbuf, and appends its boundary positions to bounds.
func (c *Chunker) readBatch() bool {
	if c.eof {
		return false
	}

	// Drop the already-emitted prefix from cbuf and the consumed boundaries.
	if c.head > 0 {
		m := copy(c.cbuf, c.cbuf[c.head:])
		c.cbuf = c.cbuf[:m]
		c.head = 0
	}
	if c.bcur > 0 {
		m := copy(c.bounds, c.bounds[c.bcur:])
		c.bounds = c.bounds[:m]
		c.bcur = 0
	}

	// Carry the previous batch's trailing window-1 bytes to the front, then fill.
	if c.carry > 0 {
		copy(c.rbuf[:c.carry], c.rbuf[c.prevN-c.carry:c.prevN])
	}
	n := c.carry
	for n < len(c.rbuf) && !c.eof {
		m, err := c.r.Read(c.rbuf[n:])
		n += m
		if err == io.EOF {
			c.eof = true
		} else if err != nil {
			c.err = err
			return false
		}
	}
	if n < c.window {
		c.eof = true
		return false
	}

	var batchO int
	if c.firstBatch {
		batchO = 0
		c.cbuf = append(c.cbuf, c.rbuf[:n]...)
		c.consumed = n
		c.firstBatch = false
	} else {
		batchO = c.consumed - (c.window - 1)
		c.cbuf = append(c.cbuf, c.rbuf[c.window-1:n]...)
		c.consumed += n - (c.window - 1)
	}
	c.appendBounds(c.rbuf[:n], batchO)

	c.carry = c.window - 1
	c.prevN = n
	return true
}

// appendBounds finds the boundary positions of one batch and appends them, as
// global boundary-byte indices, to bounds. The boundary byte for the window
// data[i:i+window] is its last byte, global index batchO+i+window-1.
func (c *Chunker) appendBounds(data []byte, batchO int) {
	w := c.window
	switch {
	case c.brd != nil:
		na, nb := c.brd.BulkBoundaries(c.la, c.lb, data, w, c.mask)
		for _, g := range c.la[:na] {
			c.bounds = append(c.bounds, batchO+int(g)+w-1)
		}
		for _, g := range c.lb[:nb] {
			c.bounds = append(c.bounds, batchO+int(g)+w-1)
		}
	case c.bulk != nil:
		ns := len(data) - w + 1
		c.bulk.BulkRoll(c.scan[:ns], data, w)
		for i := range ns {
			if c.scan[i]&c.mask == 0 {
				c.bounds = append(c.bounds, batchO+i+w-1)
			}
		}
	default:
		// Pure Roll fallback for a Hash implementing neither fast path.
		c.h.Reset()
		c.h.Write(data[:w])
		if c.sum()&c.mask == 0 {
			c.bounds = append(c.bounds, batchO+w-1)
		}
		for i := w; i < len(data); i++ {
			c.h.Roll(data[i])
			if c.sum()&c.mask == 0 {
				c.bounds = append(c.bounds, batchO+i)
			}
		}
	}
}

// fail clears the current chunk state and reports no further chunks.
func (c *Chunker) fail() bool {
	c.done = true
	c.chunk = nil
	c.sumv = 0
	c.atMask = false
	return false
}

// Chunk returns the current chunk, valid until the next call to Next. Before
// the first call to Next, and after Next returns false, Chunk returns nil.
func (c *Chunker) Chunk() []byte { return c.chunk }

// Sum returns the rolling checksum at the current chunk's boundary. Before
// the first call to Next, and after Next returns false, Sum returns 0.
func (c *Chunker) Sum() uint64 { return c.sumv }

// AtMask reports whether the current chunk was cut by the mask (true) rather
// than forced at max or at end of stream (false). Before the first call to
// Next, and after Next returns false, AtMask returns false.
func (c *Chunker) AtMask() bool { return c.atMask }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.err }
