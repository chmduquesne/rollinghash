package rollinghash

import (
	"hash"
	"io"
	"math"
)

// chunkerBatchSize is the read/hash batch the chunker uses when the hash
// implements the BatchBoundaries fast path. Kept modest so the per-batch work
// stays cache-resident.
const chunkerBatchSize = 16 << 10

// stepResult is the result of one non-blocking attempt to advance a
// splitter or batcher: whether a result was emitted, more input
// is needed before one can be, or no more results will ever come.
type stepResult int

const (
	needMore stepResult = iota
	emitted
	stepDone
)

// splitter holds all content-defined-chunking state that doesn't depend
// on how bytes arrive: the chunk accumulator, the pending boundary queue,
// and the min/max selection logic. The pull-based chunker (fed by
// chunker.fillSplitter from an io.Reader) and the push-based chunkWriter (fed
// directly by Write) each wrap one, supplying only their own "how do I get
// more bytes" mechanism.
type splitter struct {
	h      Hash
	brd    hashBoundaryRoller
	sum    func() uint64 // reads h's current sum, for windowSum
	window int
	mask   uint64
	min    int
	max    int
	la, lb []int32

	// batchSize sizes chunker's read buffer (io.Reader side) and
	// chunkWriter's write-coalescing threshold (io.Writer side): the
	// number of bytes accumulated before invoking BatchBoundaries. See
	// WithBatchSize.
	batchSize int

	// chunk byte accumulator: cbuf holds the buffered bytes for global
	// offsets [cbufBase, consumed); the byte at global offset g is
	// cbuf[g-cbufBase]. cbufBase trails chunkStart by up to window-1 bytes
	// so a rolling window straddling the previous chunk's end still has its
	// lead-in bytes available to hashForward.
	cbuf       []byte
	cbufBase   int
	chunkStart int
	consumed   int // global offset of the next not-yet-buffered byte

	bounds []int // ascending global boundary-byte positions, not yet consumed
	bcur   int

	// hashedTo is the global offset up to which BatchBoundaries has been
	// run: every rolling window ending at a byte before hashedTo has been
	// evaluated and any hit recorded in bounds. Hashing is lazy and driven
	// by next(); the [chunkStart, chunkStart+min-1) region of each chunk is
	// declared hashed without ever being fed to BatchBoundaries, since no
	// window ending there can produce a boundary the chunk would accept.
	hashedTo int

	eof bool // finish() was called: no more data will ever arrive

	done           bool
	err            error
	chunk          []byte
	sumv           uint64
	contentDefined bool
	offset         int
}

// newSplitter builds the shared boundary-finding state for both chunker
// and chunkWriter. It panics if h does not implement hashBoundaryRoller.
func newSplitter(h Hash, window int, mask uint64, min, max int) *splitter {
	brd, ok := h.(hashBoundaryRoller)
	if !ok {
		panic("rollinghash: chunker requires BatchBoundaries")
	}
	c := &splitter{
		h:         h,
		brd:       brd,
		window:    window,
		mask:      mask,
		min:       min,
		max:       max,
		batchSize: chunkerBatchSize,
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

// reset clears all buffered state for reuse with a new stream, keeping
// internal allocations (la, lb, cbuf, bounds backing arrays).
func (c *splitter) reset() {
	c.cbuf = c.cbuf[:0]
	c.cbufBase = 0
	c.chunkStart = 0
	c.consumed = 0
	c.bounds = c.bounds[:0]
	c.bcur = 0
	c.hashedTo = 0
	c.eof = false
	c.done = false
	c.err = nil
	c.chunk = nil
	c.sumv = 0
	c.contentDefined = false
	c.offset = 0
}

// finish signals that no more data will ever arrive, so next() can flush
// the trailing chunk instead of returning needMore.
func (c *splitter) finish() { c.eof = true }

// compact drops the fully-consumed prefix of cbuf and bounds so both stay
// bounded across many chunks. It runs before fresh bytes enter the
// accumulator, whether they are appended by feed (push side) or read
// straight into cbuf's tail by the pull-based chunker (see readTail).
func (c *splitter) compact() {
	c.skipPreMin()

	// Keep bytes from cbufBase = min(chunkStart, hashedTo) - (window-1): the
	// chunk accumulator needs [chunkStart, consumed) and hashForward needs
	// window-1 bytes of lead-in before its next window (which starts no
	// earlier than hashedTo, or chunkStart when a forced cut left hashedTo
	// behind). Everything before that is done with.
	keepFrom := max(min(c.chunkStart, c.hashedTo)-(c.window-1), 0)
	if drop := keepFrom - c.cbufBase; drop > 0 {
		m := copy(c.cbuf, c.cbuf[drop:])
		c.cbuf = c.cbuf[:m]
		c.cbufBase = keepFrom
	}
	if c.bcur > 0 {
		m := copy(c.bounds, c.bounds[c.bcur:])
		c.bounds = c.bounds[:m]
		c.bcur = 0
	}
}

// feed ingests newBytes (bytes not previously seen) into the chunk byte
// accumulator, compacting the already-emitted prefix first. Boundary
// detection itself is deferred to hashForward, driven lazily by next().
func (c *splitter) feed(newBytes []byte) {
	c.compact()
	c.cbuf = append(c.cbuf, newBytes...)
	c.consumed += len(newBytes)
}

// readTail compacts, then ensures cbuf has room for up to n bytes past its
// current length and returns that writable tail slice (length n). The
// caller fills the first k <= n bytes of it and then calls commitTail(k).
// This lets the pull-based chunker read from its io.Reader directly into
// the chunk accumulator instead of bouncing every byte through a separate
// read buffer.
func (c *splitter) readTail(n int) []byte {
	c.compact()
	l := len(c.cbuf)
	if cap(c.cbuf) < l+n {
		// Amortized (doubling) growth, like append, so cbuf's backing array
		// stops reallocating once it has seen a full-size chunk.
		newCap := max(cap(c.cbuf)*2, l+n)
		grown := make([]byte, l, newCap)
		copy(grown, c.cbuf)
		c.cbuf = grown
	}
	return c.cbuf[l : l+n]
}

// commitTail marks the first n bytes of the slice returned by readTail as
// filled and part of the stream.
func (c *splitter) commitTail(n int) {
	c.cbuf = c.cbuf[:len(c.cbuf)+n]
	c.consumed += n
}

// skipPreMin fast-forwards hashedTo past the current chunk's pre-min region:
// no window ending before chunkStart+min-1 can be a boundary this chunk
// accepts, and minByte only grows as chunks are emitted, so those windows
// never become relevant. This is what keeps min from being a mere
// post-filter — the skipped bytes are never fed to BatchBoundaries.
func (c *splitter) skipPreMin() {
	if skip := c.chunkStart + c.min - 1; skip > c.hashedTo {
		c.hashedTo = skip
	}
}

// hashForward runs BatchBoundaries over one batchSize-sized slice of the
// buffered-but-not-yet-hashed bytes, recording any boundary hits in bounds
// and advancing hashedTo. It first fast-forwards hashedTo past the current
// chunk's pre-min region (no window ending there can yield a boundary the
// chunk would accept, and minByte only grows as chunks are emitted, so those
// windows never become relevant). It reports whether it made progress; false
// means every buffered byte that can form a full window has been hashed.
func (c *splitter) hashForward() bool {
	w := c.window

	c.skipPreMin()
	if c.hashedTo >= c.consumed {
		return false
	}

	// window-1 bytes of context precede the first not-yet-evaluated window,
	// clamped to what's still buffered.
	lead := min(w-1, c.hashedTo-c.cbufBase)
	bufStart := c.hashedTo - lead
	end := min(c.hashedTo+c.batchSize, c.consumed)
	buf := c.cbuf[bufStart-c.cbufBase : end-c.cbufBase]
	if len(buf) < w {
		return false // not enough buffered to form another window
	}

	need := len(buf) - w + 1
	if cap(c.la) < need {
		c.la = make([]int32, need)
		c.lb = make([]int32, need)
	} else {
		c.la, c.lb = c.la[:need], c.lb[:need]
	}
	na, nb := c.brd.BatchBoundaries(c.la, c.lb, buf, w, c.mask)
	for _, g := range c.la[:na] {
		c.bounds = append(c.bounds, bufStart+int(g)+w-1)
	}
	for _, g := range c.lb[:nb] {
		c.bounds = append(c.bounds, bufStart+int(g)+w-1)
	}
	c.hashedTo = end
	return true
}

// next attempts one non-blocking chunk selection from currently buffered
// state: an in-range mask boundary, a forced cut at max, or (once finish
// has been called) the trailing bytes as a final chunk. It returns
// needMore if none of those is currently possible.
func (c *splitter) next() stepResult {
	if c.err != nil || c.done {
		c.chunk = nil
		c.sumv = 0
		c.contentDefined = false
		c.offset = 0
		return stepDone
	}

	minByte := c.chunkStart + c.min - 1 // smallest boundary byte with L >= min
	var maxByte int
	if c.max == math.MaxInt {
		maxByte = math.MaxInt // no forced cut; avoid overflow
	} else {
		maxByte = c.chunkStart + c.max - 1 // forced-cut boundary byte (L == max)
	}

	// Hash forward until a boundary that could be in range is buffered, we've
	// evaluated every window ending at or before maxByte (so a forced cut is
	// provably correct), or the buffered input is exhausted.
	hashLimit := maxByte
	if hashLimit != math.MaxInt {
		hashLimit++
	}
	for c.hashedTo < hashLimit {
		if n := len(c.bounds); n > 0 && c.bounds[n-1] >= minByte {
			break // already have a candidate the scan below can act on
		}
		if !c.hashForward() {
			break
		}
	}

	for c.bcur < len(c.bounds) {
		e := c.bounds[c.bcur]
		if e < minByte {
			c.bcur++ // too short for this chunk; never reusable
			continue
		}
		if e <= maxByte {
			c.bcur++
			c.emit(e, true)
			return emitted
		}
		break // next boundary is past max; force a cut instead
	}

	// No in-range mask boundary: force a cut at max once those bytes exist.
	if c.consumed-1 >= maxByte {
		c.emit(maxByte, false)
		return emitted
	}

	if c.eof {
		// Any bytes not yet cut form a final chunk, even a stream that
		// never reached a full window: a chunk is just a byte range, so
		// it exists regardless of whether a rolling checksum could be
		// computed over its end. windowSum reports 0 for such a chunk
		// (see Sum), and ContentDefined is false. Only a truly empty
		// stream yields no chunks.
		if c.chunkStart < c.consumed { // trailing bytes -> final chunk
			c.done = true
			c.emit(c.consumed-1, false)
			return emitted
		}
		c.done = true
		c.chunk = nil
		c.sumv = 0
		c.contentDefined = false
		c.offset = 0
		return stepDone
	}

	return needMore
}

// emit records the chunk ending at global byte e and advances past it.
func (c *splitter) emit(e int, contentDefined bool) {
	lo := c.chunkStart - c.cbufBase
	c.chunk = c.cbuf[lo : lo+(e-c.chunkStart+1)]
	c.offset = c.chunkStart
	// Sum is the rolling checksum of the window ending at the cut, regardless
	// of how the cut was chosen: at a mask boundary the caller uses it as the
	// hit value; at a forced cut or the final chunk it lets the caller confirm
	// the window did not satisfy the mask.
	c.sumv = c.windowSum(e)
	c.contentDefined = contentDefined
	c.chunkStart = e + 1
}

// windowSum recomputes the rolling checksum of the window ending at global byte
// e from the buffered bytes (cheap: once per emitted chunk). The window may
// straddle the previous chunk's end; feed keeps window-1 bytes of lead-in
// buffered before cbufBase precisely so those bytes are still available here.
// Returns 0 only when the window is not fully buffered, i.e. a final chunk
// whose stream has fewer than window bytes before its end.
func (c *splitter) windowSum(e int) uint64 {
	start := e - c.window + 1
	if start < c.cbufBase {
		return 0
	}
	off := start - c.cbufBase
	c.h.Reset()
	c.h.Write(c.cbuf[off : off+c.window])
	return c.sum()
}

// Bytes returns the current chunk, valid until the next call to next/feed.
func (c *splitter) Bytes() []byte { return c.chunk }

// Sum returns the rolling checksum of the window ending at the current chunk's
// cut, whether or not that cut was a mask hit. It is 0 only for a final chunk
// whose stream has fewer than window bytes.
func (c *splitter) Sum() uint64 { return c.sumv }

// ContentDefined reports whether the current chunk was cut by the mask.
func (c *splitter) ContentDefined() bool { return c.contentDefined }

// Err returns the first non-EOF error encountered, if any.
func (c *splitter) Err() error { return c.err }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *splitter) Offset() int { return c.offset }

// WindowSize returns the rolling window size.
func (c *splitter) WindowSize() int { return c.window }

// chunker splits an io.Reader into content-defined chunks. A boundary is
// placed after the first byte at which the rolling checksum (over the preceding
// window bytes) satisfies checksum & mask == 0, subject to a chunk length in
// [min, max]; if no such boundary is found by max, the chunk is cut at max. The
// trailing bytes of the stream form a final chunk.
//
//	c := Newchunker(r, h, window, mask, min, max)
//	for c.Next() {
//		chunk := c.Bytes()
//		if c.ContentDefined() {
//			// content-defined boundary; c.Sum() is the hit value
//		} else {
//			// forced cut at max, or the final chunk at end of stream
//		}
//	}
//	if err := c.Err(); err != nil { ... }
//
// Boundary detection is fused into the hashing loop via BatchBoundaries (no
// checksum stream is materialized). The hash must implement BatchBoundaries;
// Newchunker panics otherwise.
type chunker struct {
	sp *splitter

	r        io.Reader
	readSize int // bytes to pull per Read batch, == max(batchSize, window)
}

var _ Chunker = (*chunker)(nil)

// chunkerOption is a functional option shared by NewChunker and NewChunkWriter.
type chunkerOption func(*splitter)

// WithBoundaries sets the minimum and maximum chunk size. Chunks shorter than
// min bytes are extended to the next boundary; chunks that reach max bytes
// without a mask hit are cut there unconditionally. Defaults are 0 and
// math.MaxInt.
//
// min is not just a post-filter: the first min-window bytes of every chunk
// cannot contain an acceptable boundary, so they are skipped by the hasher
// rather than fed to BatchBoundaries. A large min therefore speeds chunking
// roughly in proportion to the fraction of the stream it covers. The skip is
// only approximate at the granularity of one hash batch (see WithBatchSize):
// the hasher may run a batch's worth of bytes past a boundary before the next
// chunk's min region is known, so gains shrink as min approaches the batch
// size.
func WithBoundaries(min, max int) chunkerOption {
	return func(c *splitter) { c.min = min; c.max = max }
}

// WithBatchSize sets the number of bytes accumulated before invoking
// BatchBoundaries: for NewChunker, the read buffer size; for
// NewChunkWriter, the write-coalescing threshold (Write defers calling
// BatchBoundaries until this many bytes have been written, or Close is
// called). A larger value means fewer, larger BatchBoundaries calls and
// better amortization of the fused fast path, at the cost of higher memory
// use and, for ChunkWriter, higher latency between Write and a chunk
// becoming available via Next. The default is 16 KiB.
//
// This is the Chunker/ChunkWriter equivalent of WithBufferSize, which
// serves the same role for BatchRoller/BatchWriter; they can't share one
// name since Go doesn't support overloading a function name across the
// two distinct option types.
func WithBatchSize(n int) chunkerOption {
	return func(c *splitter) { c.batchSize = n }
}

// WithBuffer supplies the buffer the chunker accumulates stream bytes in: it is
// filled from the reader by NewChunker, or from Write by NewChunkWriter. Its
// contents are ignored and only its capacity is used; the chunker still grows
// it if a chunk plus its hashing lead-in exceeds that capacity, so an
// undersized buffer is safe, just less effective.
//
// The accumulator's steady-state size is roughly max (the largest chunk, see
// WithBoundaries) plus one read/hash batch (WithBatchSize, default 16 KiB) plus
// window. make([]byte, 0, 2*max) covers it whenever max is at least the batch
// size; otherwise add the batch size.
//
// A chunker already keeps its buffer across Reset, so this matters only when
// you create many short-lived chunkers instead of reusing one: it lets them
// share a single allocation (for example from a sync.Pool) rather than each
// growing a buffer from nothing.
func WithBuffer(buf []byte) chunkerOption {
	return func(c *splitter) {
		if cap(buf) > cap(c.cbuf) {
			c.cbuf = buf[:0]
		}
	}
}

// NewChunker returns a chunker over r. A boundary is placed where the rolling
// checksum under h (over window bytes) satisfies checksum & mask == 0, with the
// chunk length kept in [min, max]. window must be >= 1. Use WithMinSize and
// WithMaxSize to set min (default 0) and max (default math.MaxInt).
// The hash must implement BatchBoundaries; NewChunker panics otherwise.
func NewChunker(r io.Reader, h Hash, window int, mask uint64, opts ...chunkerOption) Chunker {
	sp := newSplitter(h, window, mask, 0, math.MaxInt)
	for _, opt := range opts {
		opt(sp)
	}
	return &chunker{
		sp:       sp,
		r:        r,
		readSize: max(sp.batchSize, window),
	}
}

// Reset prepares the chunker to split r from the start, reusing its buffers.
func (c *chunker) Reset(r io.Reader) {
	c.r = r
	c.sp.reset()
}

// Next advances to the next chunk, returning false at end of input or on the
// first error. After it returns false, Err reports any error other than EOF.
func (c *chunker) Next() bool {
	for {
		switch c.sp.next() {
		case emitted:
			return true
		case stepDone:
			return false
		case needMore:
			if !c.fillSplitter() {
				if c.sp.err != nil {
					return false
				}
				// Reader exhausted; loop back so next() can flush the
				// trailing chunk (or report stepDone).
			}
		}
	}
}

// fillSplitter reads the next block from r straight into the splitter's chunk
// accumulator (no intermediate buffer) and returns false once the reader is
// exhausted (sp.finish has been called) or on error (sp.err is set).
func (c *chunker) fillSplitter() bool {
	if c.sp.eof {
		return false
	}
	buf := c.sp.readTail(c.readSize)
	n := 0
	eof := false
	for n < len(buf) && !eof {
		m, err := c.r.Read(buf[n:])
		n += m
		if err == io.EOF {
			eof = true
		} else if err != nil {
			c.sp.commitTail(n)
			c.sp.err = err
			return false
		}
	}
	c.sp.commitTail(n)
	if eof {
		c.sp.finish()
		return false
	}
	return true
}

// Bytes returns the current chunk, valid until the next call to Next. Before
// the first call to Next, and after Next returns false, Bytes returns nil.
func (c *chunker) Bytes() []byte { return c.sp.Bytes() }

// Sum returns the rolling checksum of the window ending at the current chunk's
// cut, whether the cut was a mask hit, a forced cut at max, or the end of the
// stream. It is 0 only for a final chunk whose stream has fewer than window
// bytes. Before the first call to Next, and after Next returns false, Sum
// returns 0.
func (c *chunker) Sum() uint64 { return c.sp.Sum() }

// ContentDefined reports whether the current chunk was cut by the mask (true) rather
// than forced at max or at end of stream (false). Before the first call to
// Next, and after Next returns false, ContentDefined returns false.
func (c *chunker) ContentDefined() bool { return c.sp.ContentDefined() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *chunker) Err() error { return c.sp.Err() }

// Offset returns the start byte offset of the current chunk in the stream.
// Before the first call to Next, and after Next returns false, Offset returns 0.
func (c *chunker) Offset() int { return c.sp.Offset() }

// WindowSize returns the rolling window size passed to NewChunker.
func (c *chunker) WindowSize() int { return c.sp.WindowSize() }
