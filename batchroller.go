package rollinghash

import "io"

// defaultBatchRollerBufSize is the batch buffer size used when the caller does
// not supply one via WithBufferSize. Larger batches amortize the bulk fast path
// better.
const defaultBatchRollerBufSize = 1 << 16 // 64 KiB

// batchRollerOption is a functional option for NewBatchRoller.
type batchRollerOption func(*batchRoller)

// WithBufferSize sets the internal batch buffer size in bytes. A larger buffer
// means larger batches and better amortization of the bulk fast path; it must
// be at least window bytes. The default is 64 KiB.
func WithBufferSize(n int) batchRollerOption {
	return func(s *batchRoller) { s.buf = make([]byte, n) }
}

// batchRoller walks an io.Reader and yields, in batches, the rolling checksum
// at every window position together with the bytes those checksums cover.
//
// Each Next reads a block and computes all its checksums via BatchRoll,
// carrying the trailing window-1 bytes into the next block so no window
// position is skipped or duplicated across a batch boundary. Sums() and
// Bytes() are valid only until the next call to Next. An input shorter than
// window yields no batches.
type batchRoller struct {
	r      io.Reader
	br     hashBatchRoller
	window int

	buf       []byte
	data      []byte   // current batch's bytes; == buf[:n]; nil outside a batch
	sums      []uint64 // current batch's checksums; nil outside a batch
	sumsStore []uint64 // backing array for sums; retained across Reset for reuse
	carry     int      // window-1 bytes to carry from the previous batch, or 0
	prevN     int      // length of the previous batch (where its carry tail sits)

	eof  bool // the reader has signalled io.EOF
	done bool // no more batches will be produced
	err  error
}

// NewBatchRoller returns a BatchRoller over r. window must be >= 1. h must
// implement BatchRoll; NewBatchRoller panics otherwise. Pass nil for r and
// call Reset before the first Next to defer stream attachment. Use WithBufferSize
// to control the batch size (default 64 KiB).
func NewBatchRoller(r io.Reader, h Hash, window int, opts ...batchRollerOption) BatchRoller {
	br, ok := h.(hashBatchRoller)
	if !ok {
		panic("rollinghash: BatchRoller requires BatchRoll; use Roll directly for hashes without BatchRoll")
	}
	s := &batchRoller{
		r:      r,
		br:     br,
		window: window,
		buf:    make([]byte, max(defaultBatchRollerBufSize, window)),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Reset prepares the batchRoller to roll r from the start, reusing the
// existing batch buffer and sums storage. The hash and window are unchanged.
// It lets one batchRoller process many streams without reallocating.
func (s *batchRoller) Reset(r io.Reader) {
	s.r = r
	s.data = nil
	s.sums = nil
	// sumsStore is intentionally kept to reuse its backing array.
	s.carry = 0
	s.prevN = 0
	s.eof = false
	s.done = false
	s.err = nil
}

// Next loads the next batch, returning false at end of input or on the first
// error. After it returns false, Err reports any error other than io.EOF.
func (s *batchRoller) Next() bool {
	if s.err != nil || s.done {
		s.data = nil
		s.sums = nil
		return false
	}

	// Move the previous batch's trailing window-1 bytes to the front. This is
	// deferred to here (rather than the end of the previous Next) so it does
	// not clobber the Bytes() the previous batch handed out, which stays valid
	// until exactly this call. copy is memmove-safe for the possible overlap.
	if s.carry > 0 {
		copy(s.buf[:s.carry], s.buf[s.prevN-s.carry:s.prevN])
	}

	// Fill the rest of the buffer after the carry.
	n := s.carry
	for n < len(s.buf) && !s.eof {
		m, err := s.r.Read(s.buf[n:])
		n += m
		if err == io.EOF {
			s.eof = true
		} else if err != nil {
			s.err = err
			return false
		}
	}

	if n < s.window {
		// Fewer than window bytes remain: no whole window can be formed.
		s.done = true
		s.data = nil
		s.sums = nil
		return false
	}

	s.data = s.buf[:n]
	nsums := n - s.window + 1
	if cap(s.sumsStore) < nsums {
		s.sumsStore = make([]uint64, nsums)
	}
	s.sumsStore = s.sumsStore[:nsums]
	s.sums = s.sumsStore
	s.br.BatchRoll(s.sums, s.data, s.window)

	if s.eof {
		// All remaining windows have been emitted.
		s.done = true
		s.carry = 0
	} else {
		// Remember to carry the trailing window-1 bytes into the next batch,
		// so windows straddling this boundary are produced there. The actual
		// move happens at the start of the next Next to keep this batch's
		// Bytes() intact.
		s.carry = s.window - 1
		s.prevN = n
	}
	return true
}

// Sums returns the checksums of the current batch, one per window position.
// It is valid only until the next call to Next. Before the first call to
// Next, and after Next returns false, Sums returns nil.
func (s *batchRoller) Sums() []uint64 { return s.sums }

// Bytes returns the bytes of the current batch. Sums()[i] is the checksum of
// Bytes()[i:i+window]. It is valid only until the next call to Next. Before
// the first call to Next, and after Next returns false, Bytes returns nil.
func (s *batchRoller) Bytes() []byte { return s.data }

// Err returns the first non-EOF error encountered by Next, if any.
func (s *batchRoller) Err() error { return s.err }

// WindowSize returns the rolling window size passed to NewBatchRoller.
func (s *batchRoller) WindowSize() int { return s.window }
