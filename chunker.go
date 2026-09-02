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

// coreState is the result of one non-blocking attempt to advance a
// chunkerCore or batchRollerCore: whether a result was emitted, more input
// is needed before one can be, or no more results will ever come.
type coreState int

const (
	needMore coreState = iota
	emitted
	coreDone
)

// chunkerCore holds all content-defined-chunking state that doesn't depend
// on how bytes arrive: the chunk accumulator, the pending boundary queue,
// and the min/max selection logic. The pull-based chunker (fed by
// chunker.fillCore from an io.Reader) and the push-based chunkWriter (fed
// directly by Write) each wrap one, supplying only their own "how do I get
// more bytes" mechanism.
type chunkerCore struct {
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

// newChunkerCore builds the shared boundary-finding state for both chunker
// and chunkWriter. It panics if h does not implement hashBoundaryRoller.
func newChunkerCore(h Hash, window int, mask uint64, min, max int) *chunkerCore {
	brd, ok := h.(hashBoundaryRoller)
	if !ok {
		panic("rollinghash: chunker requires BatchBoundaries")
	}
	c := &chunkerCore{
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
func (c *chunkerCore) reset() {
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
func (c *chunkerCore) finish() { c.eof = true }

// feed ingests newBytes (bytes not previously seen) into the chunk byte
// accumulator and compacts the already-emitted prefix of cbuf and bounds
// first, so both stay bounded across many chunks. Boundary detection itself
// is deferred to hashForward, driven lazily by next().
func (c *chunkerCore) feed(newBytes []byte) {
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

	c.cbuf = append(c.cbuf, newBytes...)
	c.consumed += len(newBytes)
}

// skipPreMin fast-forwards hashedTo past the current chunk's pre-min region:
// no window ending before chunkStart+min-1 can be a boundary this chunk
// accepts, and minByte only grows as chunks are emitted, so those windows
// never become relevant. This is what keeps min from being a mere
// post-filter — the skipped bytes are never fed to BatchBoundaries.
func (c *chunkerCore) skipPreMin() {
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
func (c *chunkerCore) hashForward() bool {
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
func (c *chunkerCore) next() coreState {
	if c.err != nil || c.done {
		c.chunk = nil
		c.sumv = 0
		c.contentDefined = false
		c.offset = 0
		return coreDone
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
		return coreDone
	}

	return needMore
}

// emit records the chunk ending at global byte e and advances past it.
func (c *chunkerCore) emit(e int, contentDefined bool) {
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
func (c *chunkerCore) windowSum(e int) uint64 {
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
func (c *chunkerCore) Bytes() []byte { return c.chunk }

// Sum returns the rolling checksum of the window ending at the current chunk's
// cut, whether or not that cut was a mask hit. It is 0 only for a final chunk
// whose stream has fewer than window bytes.
func (c *chunkerCore) Sum() uint64 { return c.sumv }

// ContentDefined reports whether the current chunk was cut by the mask.
func (c *chunkerCore) ContentDefined() bool { return c.contentDefined }

// Err returns the first non-EOF error encountered, if any.
func (c *chunkerCore) Err() error { return c.err }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *chunkerCore) Offset() int { return c.offset }

// WindowSize returns the rolling window size.
func (c *chunkerCore) WindowSize() int { return c.window }

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
	core *chunkerCore

	r    io.Reader
	rbuf []byte
}

// chunkerOption is a functional option shared by NewChunker and NewChunkWriter.
type chunkerOption func(*chunkerCore)

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
	return func(c *chunkerCore) { c.min = min; c.max = max }
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
	return func(c *chunkerCore) { c.batchSize = n }
}

// NewChunker returns a chunker over r. A boundary is placed where the rolling
// checksum under h (over window bytes) satisfies checksum & mask == 0, with the
// chunk length kept in [min, max]. window must be >= 1. Use WithMinSize and
// WithMaxSize to set min (default 0) and max (default math.MaxInt).
// The hash must implement BatchBoundaries; NewChunker panics otherwise.
func NewChunker(r io.Reader, h Hash, window int, mask uint64, opts ...chunkerOption) Chunker {
	core := newChunkerCore(h, window, mask, 0, math.MaxInt)
	for _, opt := range opts {
		opt(core)
	}
	bufSize := max(core.batchSize, window)
	return &chunker{
		core: core,
		r:    r,
		rbuf: make([]byte, bufSize),
	}
}

// Reset prepares the chunker to split r from the start, reusing its buffers.
func (c *chunker) Reset(r io.Reader) {
	c.r = r
	c.core.reset()
}

// Next advances to the next chunk, returning false at end of input or on the
// first error. After it returns false, Err reports any error other than EOF.
func (c *chunker) Next() bool {
	for {
		switch c.core.next() {
		case emitted:
			return true
		case coreDone:
			return false
		case needMore:
			if !c.fillCore() {
				if c.core.err != nil {
					return false
				}
				// Reader exhausted; loop back so next() can flush the
				// trailing chunk (or report coreDone).
			}
		}
	}
}

// fillCore reads the next block from r into rbuf and feeds it to core. It
// returns false once the reader is exhausted (core.finish has been called)
// or on error (core.err is set).
func (c *chunker) fillCore() bool {
	if c.core.eof {
		return false
	}
	n := 0
	eof := false
	for n < len(c.rbuf) && !eof {
		m, err := c.r.Read(c.rbuf[n:])
		n += m
		if err == io.EOF {
			eof = true
		} else if err != nil {
			c.core.err = err
			return false
		}
	}
	if n > 0 {
		c.core.feed(c.rbuf[:n])
	}
	if eof {
		c.core.finish()
		return false
	}
	return true
}

// Bytes returns the current chunk, valid until the next call to Next. Before
// the first call to Next, and after Next returns false, Bytes returns nil.
func (c *chunker) Bytes() []byte { return c.core.Bytes() }

// Sum returns the rolling checksum of the window ending at the current chunk's
// cut, whether the cut was a mask hit, a forced cut at max, or the end of the
// stream. It is 0 only for a final chunk whose stream has fewer than window
// bytes. Before the first call to Next, and after Next returns false, Sum
// returns 0.
func (c *chunker) Sum() uint64 { return c.core.Sum() }

// ContentDefined reports whether the current chunk was cut by the mask (true) rather
// than forced at max or at end of stream (false). Before the first call to
// Next, and after Next returns false, ContentDefined returns false.
func (c *chunker) ContentDefined() bool { return c.core.ContentDefined() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *chunker) Err() error { return c.core.Err() }

// Offset returns the start byte offset of the current chunk in the stream.
// Before the first call to Next, and after Next returns false, Offset returns 0.
func (c *chunker) Offset() int { return c.core.Offset() }

// WindowSize returns the rolling window size passed to NewChunker.
func (c *chunker) WindowSize() int { return c.core.WindowSize() }
