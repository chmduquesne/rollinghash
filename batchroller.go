package rollinghash

import "io"

// defaultBatchRollerBufSize is the batch buffer size used when the caller does
// not supply one via WithBufferSize. Larger batches amortize the bulk fast path
// better.
const defaultBatchRollerBufSize = 1 << 16 // 64 KiB

// batchRollerOption is a functional option shared by NewBatchRoller and
// NewBatchWriter.
type batchRollerOption func(*batchRollerCore)

// WithBufferSize sets the batch size in bytes: how many bytes worth of
// checksums are computed per Next(). A larger value means larger batches and
// better amortization of the bulk fast path; it must be at least window
// bytes. The default is 64 KiB.
func WithBufferSize(n int) batchRollerOption {
	return func(c *batchRollerCore) { c.batchSize = n }
}

// batchRollerCore holds all rolling-checksum batching state that doesn't
// depend on how bytes arrive: the byte accumulator and the logic that turns
// it into aligned (Bytes, Sums) batches. batchRoller (pull, io.Reader) and
// batchWriter (push, io.Writer) are both thin wrappers around one.
type batchRollerCore struct {
	br        hashBatchRoller
	window    int
	batchSize int

	buf       []byte   // accumulated, not-yet-emitted bytes (includes any pending carry)
	carry     int      // trailing window-1 bytes of the last emitted batch, still in buf
	prevN     int      // length of the last emitted batch (where its carry tail sits)

	data      []byte   // current batch's bytes; == buf[:n]; nil outside a batch
	sums      []uint64 // current batch's checksums; nil outside a batch
	sumsStore []uint64 // backing array for sums; retained across reset for reuse
	offset    int      // stream position of Bytes()[0] in the current batch

	eof  bool // finish() was called: no more data will ever arrive
	done bool
	err  error
}

func newBatchRollerCore(h Hash, window, batchSize int) *batchRollerCore {
	br, ok := h.(hashBatchRoller)
	if !ok {
		panic("rollinghash: BatchRoller requires BatchRoll; use Roll directly for hashes without BatchRoll")
	}
	return &batchRollerCore{
		br:        br,
		window:    window,
		batchSize: max(batchSize, window),
	}
}

// reset clears all buffered state for reuse with a new stream, keeping
// internal allocations (buf, sumsStore).
func (c *batchRollerCore) reset() {
	c.buf = c.buf[:0]
	c.carry = 0
	c.prevN = 0
	c.data = nil
	c.sums = nil
	// sumsStore is intentionally kept to reuse its backing array.
	c.offset = 0
	c.eof = false
	c.done = false
	c.err = nil
}

// finish signals that no more data will ever arrive, so next() can emit the
// final, possibly short, batch instead of returning needMore.
func (c *batchRollerCore) finish() { c.eof = true }

// feed appends newBytes (bytes not previously seen) to the accumulator.
func (c *batchRollerCore) feed(newBytes []byte) {
	c.buf = append(c.buf, newBytes...)
}

// next attempts one non-blocking batch emission from currently buffered
// state. It returns needMore if fewer than window bytes are available yet
// and finish hasn't been called.
func (c *batchRollerCore) next() coreState {
	if c.err != nil || c.done {
		c.data = nil
		c.sums = nil
		c.offset = 0
		return coreDone
	}

	// Advance past the new bytes yielded by the previous batch (all but the
	// window-1 carry bytes that overlap into this one), then drop the
	// already-emitted prefix from buf. This is deferred to here (rather than
	// the end of the previous next()) so it does not clobber the Bytes()/
	// Sums() the previous batch handed out, which stay valid until exactly
	// this call.
	if c.prevN > 0 {
		c.offset += c.prevN - c.carry
		// Open-ended: keeps both the carry-back region and any bytes beyond
		// prevN that arrived but weren't part of the last processed batch
		// (buf can hold more than batchSize bytes when a feed lands on top
		// of a still-pending carry).
		m := copy(c.buf, c.buf[c.prevN-c.carry:])
		c.buf = c.buf[:m]
		c.prevN = 0
	}

	if len(c.buf) < c.window {
		if c.eof {
			c.done = true
			c.data = nil
			c.sums = nil
			c.offset = 0
			return coreDone
		}
		return needMore
	}

	n := min(len(c.buf), c.batchSize)
	c.data = c.buf[:n]
	nsums := n - c.window + 1
	if cap(c.sumsStore) < nsums {
		c.sumsStore = make([]uint64, nsums)
	}
	c.sumsStore = c.sumsStore[:nsums]
	c.sums = c.sumsStore
	c.br.BatchRoll(c.sums, c.data, c.window)

	if c.eof && n == len(c.buf) {
		// All remaining windows have been emitted.
		c.done = true
		c.carry = 0
	} else {
		c.carry = c.window - 1
	}
	c.prevN = n
	return emitted
}

// Bytes returns the bytes of the current batch, valid until the next call
// to Next.
func (c *batchRollerCore) Bytes() []byte { return c.data }

// Sums returns the checksums of the current batch, one per window position.
func (c *batchRollerCore) Sums() []uint64 { return c.sums }

// Err returns the first non-EOF error encountered, if any.
func (c *batchRollerCore) Err() error { return c.err }

// Offset returns the stream position of Bytes()[0] in the current batch.
func (c *batchRollerCore) Offset() int { return c.offset }

// WindowSize returns the rolling window size.
func (c *batchRollerCore) WindowSize() int { return c.window }

// batchRoller walks an io.Reader and yields, in batches, the rolling checksum
// at every window position together with the bytes those checksums cover.
//
// Each Next reads a block and computes all its checksums via BatchRoll,
// carrying the trailing window-1 bytes into the next block so no window
// position is skipped or duplicated across a batch boundary. Sums() and
// Bytes() are valid only until the next call to Next. An input shorter than
// window yields no batches.
type batchRoller struct {
	core *batchRollerCore

	r    io.Reader
	rbuf []byte
}

// NewBatchRoller returns a BatchRoller over r. window must be >= 1. h must
// implement BatchRoll; NewBatchRoller panics otherwise. Pass nil for r and
// call Reset before the first Next to defer stream attachment. Use WithBufferSize
// to control the batch size (default 64 KiB).
func NewBatchRoller(r io.Reader, h Hash, window int, opts ...batchRollerOption) BatchRoller {
	core := newBatchRollerCore(h, window, defaultBatchRollerBufSize)
	for _, opt := range opts {
		opt(core)
	}
	return &batchRoller{
		core: core,
		r:    r,
		rbuf: make([]byte, core.batchSize),
	}
}

// Reset prepares the batchRoller to roll r from the start, reusing the
// existing batch buffer and sums storage. The hash and window are unchanged.
// It lets one batchRoller process many streams without reallocating.
func (s *batchRoller) Reset(r io.Reader) {
	s.r = r
	s.core.reset()
}

// Next loads the next batch, returning false at end of input or on the first
// error. After it returns false, Err reports any error other than io.EOF.
func (s *batchRoller) Next() bool {
	for {
		switch s.core.next() {
		case emitted:
			return true
		case coreDone:
			return false
		case needMore:
			if !s.fillCore() {
				if s.core.err != nil {
					return false
				}
				// Reader exhausted; loop back so next() can emit the final
				// batch (or report coreDone).
			}
		}
	}
}

// fillCore reads the next block from r into rbuf and feeds it to core. It
// returns false once the reader is exhausted (core.finish has been called)
// or on error (core.err is set).
func (s *batchRoller) fillCore() bool {
	if s.core.eof {
		return false
	}
	n := 0
	eof := false
	for n < len(s.rbuf) && !eof {
		m, err := s.r.Read(s.rbuf[n:])
		n += m
		if err == io.EOF {
			eof = true
		} else if err != nil {
			s.core.err = err
			return false
		}
	}
	if n > 0 {
		s.core.feed(s.rbuf[:n])
	}
	if eof {
		s.core.finish()
		return false
	}
	return true
}

// Sums returns the checksums of the current batch, one per window position.
// It is valid only until the next call to Next. Before the first call to
// Next, and after Next returns false, Sums returns nil.
func (s *batchRoller) Sums() []uint64 { return s.core.Sums() }

// Bytes returns the bytes of the current batch. Sums()[i] is the checksum of
// Bytes()[i:i+window]. It is valid only until the next call to Next. Before
// the first call to Next, and after Next returns false, Bytes returns nil.
func (s *batchRoller) Bytes() []byte { return s.core.Bytes() }

// Err returns the first non-EOF error encountered by Next, if any.
func (s *batchRoller) Err() error { return s.core.Err() }

// Offset returns the stream position of Bytes()[0] in the current batch.
// Sums()[i] is the checksum of the window starting at Offset()+i.
// Before the first call to Next, and after Next returns false, Offset returns 0.
func (s *batchRoller) Offset() int { return s.core.Offset() }

// WindowSize returns the rolling window size passed to NewBatchRoller.
func (s *batchRoller) WindowSize() int { return s.core.WindowSize() }
