// Package jumpchunker splits a stream into content-defined chunks using the
// Jump Chunking (JC) algorithm.
package jumpchunker

import (
	"io"
	"math/bits"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
)

// batchSize is the read/hash batch size, matching rollinghash's chunker
// package default.
const batchSize = 16 << 10

// jumpBoundaryRoller is a windowless CDC fast path using the Jump Chunking
// algorithm. JumpBoundaries scans data starting at firstSkip (the remaining
// min-zone bytes for the current chunk; fp is 0 in that zone). After each
// boundary it advances minStep bytes before resuming (the next chunk's min
// zone); after a false maskJ hit it advances jumpLen bytes. It returns boundary
// positions in a[:n], the updated fingerprint newFp, and skip: bytes to skip at
// the start of the next data slice when a jump or min-step crossed the batch
// boundary (fp is 0 in that case).
type jumpBoundaryRoller interface {
	JumpBoundaries(a []int32, data []byte, maskC uint64, jumpLen int, fp uint64, firstSkip, minStep int) (n int, newFp uint64, skip int)
}

// A Chunker splits an io.Reader into content-defined chunks using the
// Jump Chunking (JC) algorithm. Unlike rollinghash.Chunker, JC uses a
// windowless accumulating fingerprint (fp = (fp<<1) + G[b]) with a dual-mask
// trick that skips regions provably free of boundaries, achieving higher
// throughput than rollinghash.Chunker at the cost of producing different
// boundaries.
//
//	c := jumpchunker.New(r, gearhash64.New(), normalSize, min, max)
//	for c.Next() {
//		chunk := c.Bytes()
//		if c.AtMask() {
//			// content-defined boundary
//		} else {
//			// forced cut at max, or the final chunk at end of stream
//		}
//	}
//	if err := c.Err(); err != nil { ... }
//
// The hash must implement JumpBoundaries; New panics otherwise. Use Reset to
// reuse the Chunker across streams without extra allocations.
type Chunker struct {
	jbrd    jumpBoundaryRoller
	r       io.Reader
	maskC   uint64
	jumpLen int
	min     int
	max     int

	// cbuf accumulates raw bytes. Its spare capacity is used as the read
	// target to avoid a separate rbuf→cbuf copy each batch.
	cbuf       []byte
	head       int
	chunkStart int
	consumed   int

	jfp   uint64 // accumulated fingerprint across batch boundaries
	jskip int    // bytes to skip at start of next batch (pending jump or min-step)

	bounds []int
	bcur   int
	la     []int32

	eof    bool
	done   bool
	err    error
	chunk  []byte
	atMask bool
}

// Option is a functional option for New.
type Option func(*Chunker)

// WithJumpMask overrides the maskC and jumpLen that would otherwise be derived
// from normalSize. Use this to interoperate with another implementation that
// fixes its own boundary mask and jump stride.
func WithJumpMask(maskC uint64, jumpLen int) Option {
	return func(c *Chunker) {
		c.maskC = maskC
		c.jumpLen = jumpLen
	}
}

// New returns a Chunker over r. normalSize is the target average chunk
// length; the JC algorithm internally derives the boundary mask and jump
// length from it to maximize throughput at that target. Chunk lengths are
// kept in [min, max]. h must implement JumpBoundaries; New panics otherwise.
// Options (e.g. WithJumpMask) can override derived parameters.
func New(r io.Reader, h rollinghash.Hash, normalSize, min, max int, opts ...Option) *Chunker {
	jbrd, ok := h.(jumpBoundaryRoller)
	if !ok {
		panic("jumpchunker: Chunker requires JumpBoundaries")
	}
	maskC, jumpLen := jumpParams(normalSize)
	c := &Chunker{
		jbrd:    jbrd,
		r:       r,
		maskC:   maskC,
		jumpLen: jumpLen,
		min:     min,
		max:     max,
		cbuf:    make([]byte, 0, max+batchSize),
		la:      make([]int32, batchSize),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// jumpParams derives the maskC and jumpLen for a given target normalSize.
//
// bits = floor(log2(normalSize)); cOnes = bits-2 set bits in maskC;
// jumpLen = 2^(bits-1). This gives a 1/5 byte-examination rate: maskJ (one
// fewer bit than maskC) fires every 2^(cOnes-1) examined bytes, and each fire
// skips jumpLen bytes, so the fraction examined = 2^(cOnes-1) /
// (2^(cOnes-1)+jumpLen) = 2^(bits-3)/(2^(bits-3)+2^(bits-1)) = 1/5.
func jumpParams(normalSize int) (maskC uint64, jumpLen int) {
	lg := bits.Len(uint(normalSize)) - 1 // floor(log2(normalSize))
	if lg < 3 {
		lg = 3
	}
	cOnes := lg - 2
	jumpLen = 1 << (lg - 1)
	maskC = jumpMask(cOnes)
	return
}

// jumpMask builds a uint64 with exactly cOnes set bits, evenly spaced from
// bit 63 downward with step = 64/cOnes.
func jumpMask(cOnes int) uint64 {
	step := 64 / cOnes
	var mask uint64
	for i := 0; i < cOnes; i++ {
		mask |= 1 << uint(63-i*step)
	}
	return mask
}

// Reset prepares the Chunker to split r from the start, reusing its buffers.
func (c *Chunker) Reset(r io.Reader) {
	c.r = r
	c.jfp = 0
	c.jskip = 0
	c.cbuf = c.cbuf[:0]
	c.head = 0
	c.chunkStart = 0
	c.consumed = 0
	c.bounds = c.bounds[:0]
	c.bcur = 0
	c.eof = false
	c.done = false
	c.err = nil
	c.chunk = nil
	c.atMask = false
}

// Next advances to the next chunk, returning false at end of input or on the
// first error. After it returns false, Err reports any error other than EOF.
func (c *Chunker) Next() bool {
	if c.err != nil || c.done {
		c.chunk = nil
		c.atMask = false
		return false
	}
	for {
		minByte := c.chunkStart + c.min - 1
		maxByte := c.chunkStart + c.max - 1

		for c.bcur < len(c.bounds) {
			e := c.bounds[c.bcur]
			if e < minByte {
				c.bcur++
				continue
			}
			if e <= maxByte {
				c.bcur++
				return c.emit(e, true)
			}
			break
		}

		if c.consumed-1 >= maxByte {
			return c.emit(maxByte, false)
		}

		if !c.readBatch() {
			if c.err != nil {
				return c.jFail()
			}
			if c.head < len(c.cbuf) {
				c.done = true
				return c.emit(c.consumed-1, false)
			}
			return c.jFail()
		}
	}
}

// emit records the chunk ending at global byte e.
func (c *Chunker) emit(e int, atMask bool) bool {
	l := e - c.chunkStart + 1
	c.chunk = c.cbuf[c.head : c.head+l]
	c.atMask = atMask
	c.head += l
	c.chunkStart += l
	return true
}

// readBatch reads the next block directly into cbuf's spare capacity,
// finds jump-chunk boundaries within it, and records them in c.bounds.
func (c *Chunker) readBatch() bool {
	// Compact delivered bytes to reclaim the front of cbuf.
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

	if c.eof {
		return false
	}

	// Grow cbuf if needed (does not happen when cap was pre-allocated correctly).
	if cap(c.cbuf)-len(c.cbuf) < batchSize {
		newbuf := make([]byte, len(c.cbuf), len(c.cbuf)+batchSize)
		copy(newbuf, c.cbuf)
		c.cbuf = newbuf
	}

	// Read directly into cbuf's spare capacity, skipping the rbuf→cbuf copy.
	readBase := len(c.cbuf)
	c.cbuf = c.cbuf[:readBase+batchSize]
	n := 0
	for n < batchSize && !c.eof {
		m, err := c.r.Read(c.cbuf[readBase+n : readBase+batchSize])
		n += m
		if err == io.EOF {
			c.eof = true
		} else if err != nil {
			c.err = err
			c.cbuf = c.cbuf[:readBase]
			return false
		}
	}
	c.cbuf = c.cbuf[:readBase+n]
	if n == 0 {
		return false
	}

	if c.jskip >= n {
		// Entire batch falls within a pending skip; accumulate bytes into cbuf
		// but defer boundary scan.
		c.consumed += n
		c.jskip -= n
		return true
	}

	batchO := c.consumed
	oldSkip := c.jskip
	c.consumed += n

	// firstSkip: bytes at the start of the scanned slice that belong to the
	// current chunk's min zone. JumpBoundaries treats these as fp=0 and emits
	// no boundaries there.
	firstSkip := c.chunkStart + c.min - (batchO + oldSkip)
	if firstSkip < 0 {
		firstSkip = 0
	}

	nb, newFp, skip := c.jbrd.JumpBoundaries(c.la, c.cbuf[readBase+oldSkip:readBase+n], c.maskC, c.jumpLen, c.jfp, firstSkip, c.min)
	c.jfp = newFp
	c.jskip = skip

	for _, g := range c.la[:nb] {
		c.bounds = append(c.bounds, batchO+oldSkip+int(g))
	}
	return true
}

// jFail clears state and signals no further chunks.
func (c *Chunker) jFail() bool {
	c.done = true
	c.chunk = nil
	c.atMask = false
	return false
}

// Bytes returns the current chunk, valid until the next call to Next. Before
// the first call to Next, and after Next returns false, Bytes returns nil.
func (c *Chunker) Bytes() []byte { return c.chunk }

// AtMask reports whether the current chunk was cut by the mask (true) or
// forced at max / end of stream (false). Before the first call to Next, and
// after Next returns false, AtMask returns false.
func (c *Chunker) AtMask() bool { return c.atMask }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.err }
