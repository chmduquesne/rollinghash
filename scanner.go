package rollinghash

import (
	"hash"
	"io"
)

// defaultScannerBufSize is the batch buffer size used when the caller does
// not supply one with Buffer. Larger batches amortize the bulk fast path
// better.
const defaultScannerBufSize = 1 << 16 // 64 KiB

// A Scanner walks an io.Reader and yields, in batches, the rolling checksum
// at every window position together with the bytes those checksums cover. It
// is shaped like bufio.Scanner:
//
//	s := NewScanner(r, h, window)
//	for s.Scan() {
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
// Each Scan reads a block, computes all its checksums (via BulkRoller when
// the hash implements it, otherwise Write+Roll), and carries the trailing
// window-1 bytes into the next block so no window position is skipped or
// duplicated across a batch boundary. Sums() and Bytes() are valid only
// until the next call to Scan. An input shorter than window yields no
// batches.
type Scanner struct {
	r      io.Reader
	h      Hash
	br     BulkRoller // non-nil when h implements the fast path
	window int

	buf   []byte
	data  []byte   // current batch's bytes; == buf[:n]
	sums  []uint64 // current batch's checksums
	carry int      // window-1 bytes to carry from the previous batch, or 0
	prevN int      // length of the previous batch (where its carry tail sits)

	sum  func() uint64 // reads h's current sum; fallback path only
	eof  bool          // the reader has signalled io.EOF
	done bool          // no more batches will be produced
	err  error
}

// NewScanner returns a Scanner over r that produces, for every window-sized
// slice of the stream, its rolling checksum under h. window must be >= 1.
func NewScanner(r io.Reader, h Hash, window int) *Scanner {
	br, _ := h.(BulkRoller)
	s := &Scanner{
		r:      r,
		h:      h,
		br:     br,
		window: window,
		buf:    make([]byte, max(defaultScannerBufSize, window)),
	}
	if br == nil {
		// Resolve a non-allocating sum reader once. h.Sum(buf) would escape a
		// fresh buffer on every position, so prefer Sum64/Sum32 when present.
		switch v := h.(type) {
		case hash.Hash64:
			s.sum = v.Sum64
		case hash.Hash32:
			s.sum = func() uint64 { return uint64(v.Sum32()) }
		default:
			// Sum appends to a buffer; capture one in the closure so it is
			// allocated once, not on every position.
			var scratch [8]byte
			s.sum = func() uint64 {
				var res uint64
				for _, b := range h.Sum(scratch[:0]) {
					res = res<<8 | uint64(b)
				}
				return res
			}
		}
	}
	return s
}

// Buffer sets the buffer used to hold each batch. It must be called before
// the first call to Scan, and buf must be at least window bytes long. A
// larger buffer means larger batches and better amortization of the bulk
// fast path. By default Scanner allocates its own buffer.
func (s *Scanner) Buffer(buf []byte) {
	s.buf = buf
}

// Reset prepares the Scanner to scan r from the start, reusing the existing
// batch buffer and sums storage. The hash and window are unchanged. It lets
// one Scanner process many streams without reallocating.
func (s *Scanner) Reset(r io.Reader) {
	s.r = r
	s.data = nil
	s.sums = s.sums[:0]
	s.carry = 0
	s.prevN = 0
	s.eof = false
	s.done = false
	s.err = nil
}

// Scan loads the next batch, returning false at end of input or on the first
// error. After it returns false, Err reports any error other than io.EOF.
func (s *Scanner) Scan() bool {
	if s.err != nil || s.done {
		return false
	}

	// Move the previous batch's trailing window-1 bytes to the front. This is
	// deferred to here (rather than the end of the previous Scan) so it does
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
		return false
	}

	s.data = s.buf[:n]
	nsums := n - s.window + 1
	if cap(s.sums) < nsums {
		s.sums = make([]uint64, nsums)
	}
	s.sums = s.sums[:nsums]
	s.bulkRoll(s.sums, s.data, s.window)

	if s.eof {
		// All remaining windows have been emitted.
		s.done = true
		s.carry = 0
	} else {
		// Remember to carry the trailing window-1 bytes into the next batch,
		// so windows straddling this boundary are produced there. The actual
		// move happens at the start of the next Scan to keep this batch's
		// Bytes() intact.
		s.carry = s.window - 1
		s.prevN = n
	}
	return true
}

// bulkRoll fills dst with the rolling checksum at every window position of
// data, using the hash's BulkRoller fast path when available and falling
// back to Write+Roll otherwise.
func (s *Scanner) bulkRoll(dst []uint64, data []byte, window int) {
	if s.br != nil {
		s.br.BulkRoll(dst, data, window)
		return
	}
	s.h.Reset()
	s.h.Write(data[:window])
	dst[0] = s.sum()
	for i := window; i < len(data); i++ {
		s.h.Roll(data[i])
		dst[i-window+1] = s.sum()
	}
}

// Sums returns the checksums of the current batch, one per window position.
// It is valid only until the next call to Scan.
func (s *Scanner) Sums() []uint64 { return s.sums }

// Bytes returns the bytes of the current batch. Sums()[i] is the checksum of
// Bytes()[i:i+window]. It is valid only until the next call to Scan.
func (s *Scanner) Bytes() []byte { return s.data }

// Err returns the first non-EOF error encountered by Scan, if any.
func (s *Scanner) Err() error { return s.err }
