package rollinghash

import "io"

// defaultBatchRollerBufSize is the batch buffer size used when the caller does
// not supply one with Buffer. Larger batches amortize the bulk fast path
// better.
const defaultBatchRollerBufSize = 1 << 16 // 64 KiB

// hashBatchRoller walks an io.Reader and yields, in batches, the rolling
// checksum at every window position together with the bytes those checksums
// cover. Call Next to advance to the next batch, then read Sums and Bytes:
//
//	s := NewBatchRoller(r, h, window)
//	for s.Next() {
//		sums, data := s.Sums(), s.Bytes()
//		for i, sum := range sums {
//			// sum is the rolling checksum of data[i:i+window]
//		}
//	}
//	if err := s.Err(); err != nil { ... }
//
// The design rests on one alignment guarantee, per batch:
//
//	Sums()[i] is the rolling checksum of the window Bytes()[i:i+window],
//	and len(Sums()) == len(Bytes()) - window + 1.
//
// Each Next reads a block and computes all its checksums via BatchRoll,
// carrying the trailing window-1 bytes into the next block so no window
// position is skipped or duplicated across a batch boundary. Sums() and
// Bytes() are valid only until the next call to Next. An input shorter than
// window yields no batches. The hash must implement BatchRoll; NewBatchRoller
// panics otherwise.
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

// NewBatchRoller returns a batchRoller over r that produces, for every
// window-sized slice of the stream, its rolling checksum under h. window
// must be >= 1. h must implement BatchRoll; NewBatchRoller panics otherwise.
func NewBatchRoller(r io.Reader, h Hash, window int) BatchRoller {
	br, ok := h.(hashBatchRoller)
	if !ok {
		panic("rollinghash: BatchRoller requires BatchRoll; use Roll directly for hashes without BatchRoll")
	}
	return &batchRoller{
		r:      r,
		br:     br,
		window: window,
		buf:    make([]byte, max(defaultBatchRollerBufSize, window)),
	}
}

// Buffer sets the buffer used to hold each batch. It must be called before
// the first call to Next, and buf must be at least window bytes long. A
// larger buffer means larger batches and better amortization of the bulk
// fast path. By default batchRoller allocates its own buffer.
func (s *batchRoller) Buffer(buf []byte) {
	s.buf = buf
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
