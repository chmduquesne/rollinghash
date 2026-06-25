package rollinghash_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"testing/iotest"

	"github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/adler32"
)

// noBulkRoller hides BulkRoll (forcing the Scanner's Write+Roll fallback) but
// still exposes Sum32 - like a typical hash that simply hasn't implemented the
// fast path. Embedding promotes only the Hash interface's methods, so BulkRoll
// is hidden; Sum32 is forwarded explicitly so the Scanner uses its Hash32 sum
// reader.
type noBulkRoller struct{ rollinghash.Hash }

func (n noBulkRoller) Sum32() uint32 {
	return n.Hash.(interface{ Sum32() uint32 }).Sum32()
}

// sumOnly additionally hides Sum64, exercising the Scanner's generic byte-wise
// sum reader (the default branch of the sum-reader switch).
type sumOnly struct{ rollinghash.Hash }

// scannerHashes extends allHashes with a Write+Roll fallback entry (a hash that
// hides BulkRoll) so the Scanner's slow path is exercised alongside every
// BulkRoller implementation.
var scannerHashes = func() []struct {
	name string
	new  func() rollinghash.Hash
} {
	type E = struct {
		name string
		new  func() rollinghash.Hash
	}
	out := make([]E, 0, len(allHashes)+1)
	for _, h := range allHashes {
		out = append(out, E{h.name, h.new})
	}
	return append(out, E{"fallback", func() rollinghash.Hash { return noBulkRoller{adler32.New()} }})
}()

// collectScannerSums runs a Scanner to exhaustion and returns the
// concatenation of every batch's Sums(), checking the per-batch alignment
// invariant (len(Sums) == len(Bytes)-window+1) along the way.
func collectScannerSums(t *testing.T, name string, s *rollinghash.Scanner, window int) []uint64 {
	t.Helper()
	var got []uint64
	for s.Scan() {
		sums, data := s.Sums(), s.Bytes()
		if len(sums) != len(data)-window+1 {
			t.Fatalf("[%s] alignment: len(Sums)=%d len(Bytes)=%d window=%d",
				name, len(sums), len(data), window)
		}
		got = append(got, sums...)
	}
	if err := s.Err(); err != nil {
		t.Fatalf("[%s] Err: %v", name, err)
	}
	return got
}

func equalSums(t *testing.T, name string, got, want []uint64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("[%s] got %d sums, want %d", name, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%s] sum %d: got 0x%x want 0x%x", name, i, got[i], want[i])
		}
	}
}

// TestScanner checks that the concatenated Sums() over a multi-batch stream
// equal the classic hash of every window position.
func TestScanner(t *testing.T) {
	// Larger than the default 64 KiB buffer, so several batches are produced.
	data := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 5000)
	const window = 64
	for _, h := range scannerHashes {
		want := bulkRollOracle(h.new(), data, window)
		s := rollinghash.NewScanner(bytes.NewReader(data), h.new(), window)
		got := collectScannerSums(t, h.name, s, window)
		equalSums(t, h.name, got, want)
	}
}

// TestScannerBatchBoundaries is the load-bearing test: a window that straddles
// a Scan() boundary must produce the same checksum as if the input had never
// been split. It feeds the same input through adversarial buffer sizes (and a
// one-byte-at-a-time reader to stress the refill loop) and compares against a
// single oracle.
func TestScannerBatchBoundaries(t *testing.T) {
	data := make([]byte, 500)
	for i := range data {
		data[i] = byte(i*73 + 11)
	}
	const window = 16
	bufSizes := []int{window, window + 1, window + 7, 2 * window, 3*window - 1, 97, len(data)}

	for _, h := range scannerHashes {
		want := bulkRollOracle(h.new(), data, window)
		for _, bs := range bufSizes {
			// Plain reader.
			s := rollinghash.NewScanner(bytes.NewReader(data), h.new(), window)
			s.Buffer(make([]byte, bs))
			got := collectScannerSums(t, h.name, s, window)
			equalSums(t, h.name, got, want)

			// One byte per Read, to exercise the buffer-fill loop.
			s = rollinghash.NewScanner(iotest.OneByteReader(bytes.NewReader(data)), h.new(), window)
			s.Buffer(make([]byte, bs))
			got = collectScannerSums(t, h.name, s, window)
			equalSums(t, h.name, got, want)
		}
	}
}

func testData(n int) []byte {
	data := make([]byte, n)
	// A mix that produces a realistic spread of values.
	for i := range data {
		data[i] = byte(i*2654435761 + i/7)
	}
	return data
}

// TestScannerBytes verifies that each batch's Bytes() holds the exact source
// bytes for that batch - across batch boundaries and adversarial buffer sizes.
// (Sums()-only tests miss a buffer that corrupts the bytes it hands back.)
func TestScannerBytes(t *testing.T) {
	data := testData(5000)
	const window = 16
	for _, bs := range []int{window, window + 1, 64, 333, len(data)} {
		s := rollinghash.NewScanner(bytes.NewReader(data), allHashes[0].new(), window)
		s.Buffer(make([]byte, bs))
		off := 0
		for s.Scan() {
			b := s.Bytes()
			if !bytes.Equal(b, data[off:off+len(b)]) {
				t.Fatalf("buf=%d: batch at offset %d does not match source", bs, off)
			}
			off += len(b) - (window - 1) // batches overlap by window-1
		}
		if err := s.Err(); err != nil {
			t.Fatalf("buf=%d: Err %v", bs, err)
		}
	}
}

// TestScannerShortInput verifies that an input shorter than window yields no
// batches and zero-value accessors, while exactly window bytes yields one sum.
func TestScannerShortInput(t *testing.T) {
	const window = 16
	for _, h := range scannerHashes {
		for _, n := range []int{0, 1, window - 1} {
			s := rollinghash.NewScanner(bytes.NewReader(make([]byte, n)), h.new(), window)
			if s.Scan() {
				t.Errorf("[%s] n=%d: expected no batch", h.name, n)
			}
			if s.Err() != nil {
				t.Errorf("[%s] n=%d: unexpected Err %v", h.name, n, s.Err())
			}
			if s.Sums() != nil || s.Bytes() != nil {
				t.Errorf("[%s] n=%d: expected nil Sums/Bytes before any batch", h.name, n)
			}
		}
		// Exactly window bytes: one batch with a single sum.
		s := rollinghash.NewScanner(bytes.NewReader(make([]byte, window)), h.new(), window)
		got := collectScannerSums(t, h.name, s, window)
		if len(got) != 1 {
			t.Errorf("[%s] exactly window: got %d sums, want 1", h.name, len(got))
		}
	}
}

// TestScannerSumOnlyFallback covers the Scanner's generic byte-wise sum reader
// (used when the hash implements neither BulkRoller nor Sum64/Sum32).
func TestScannerSumOnlyFallback(t *testing.T) {
	data := make([]byte, 300)
	for i := range data {
		data[i] = byte(i*53 + 3)
	}
	const window = 20
	h := allHashes[0].new()
	want := bulkRollOracle(h, data, window)
	s := rollinghash.NewScanner(bytes.NewReader(data), sumOnly{allHashes[0].new()}, window)
	got := collectScannerSums(t, "sumOnly", s, window)
	equalSums(t, "sumOnly", got, want)
}

// TestScannerError verifies that a reader error is surfaced through Err and
// stops the scan.
func TestScannerError(t *testing.T) {
	boom := errors.New("boom")
	for _, h := range scannerHashes {
		s := rollinghash.NewScanner(iotest.ErrReader(boom), h.new(), 16)
		if s.Scan() {
			t.Errorf("[%s] expected Scan to fail on reader error", h.name)
		}
		if !errors.Is(s.Err(), boom) {
			t.Errorf("[%s] expected Err to be boom, got %v", h.name, s.Err())
		}
	}
}

// FuzzScanner cross-checks the Scanner against the oracle on random data,
// window sizes, and buffer sizes, using both the BulkRoller fast path and the
// Write+Roll fallback. It verifies three invariants per batch:
//   - alignment: len(Sums()) == len(Bytes()) - window + 1
//   - bytes: Bytes() matches the corresponding slice of the original input
//   - sums: concatenated Sums() equals the classic per-window hash (the oracle)
func FuzzScanner(f *testing.F) {
	f.Add([]byte("The quick brown fox jumps over the lazy dog"), 4, 16)
	f.Add(testData(9000), 16, 64)
	f.Add([]byte("hello"), 1, 1)

	f.Fuzz(func(t *testing.T, data []byte, window int, bufSize int) {
		if len(data) == 0 || window < 1 || window > len(data) {
			return
		}
		if bufSize < window {
			bufSize = window
		}
		if bufSize > window+(1<<16) {
			bufSize = window + (1 << 16)
		}

		for _, hc := range scannerHashes {
			want := bulkRollOracle(hc.new(), data, window)
			h := hc.new()

			s := rollinghash.NewScanner(bytes.NewReader(data), h, window)
			s.Buffer(make([]byte, bufSize))

			var got []uint64
			off := 0
			for s.Scan() {
				b, sums := s.Bytes(), s.Sums()
				if len(sums) != len(b)-window+1 {
					t.Fatalf("[%s] alignment: len(Sums)=%d len(Bytes)=%d window=%d",
						hc.name, len(sums), len(b), window)
				}
				if !bytes.Equal(b, data[off:off+len(b)]) {
					t.Fatalf("[%s] Bytes() at offset %d does not match source", hc.name, off)
				}
				got = append(got, sums...)
				off += len(b) - (window - 1)
			}
			if err := s.Err(); err != nil {
				t.Fatalf("[%s] Err: %v", hc.name, err)
			}
			equalSums(t, hc.name, got, want)
		}
	})
}

// BenchmarkScanner measures steady-state throughput, reusing one Scanner (and
// its buffers) across iterations via Reset so the numbers reflect scanning,
// not per-stream setup. It covers every BulkRoller implementation (bozo64,
// buzhash64, gearhash64, rabinkarp64) plus the Write+Roll fallback, across
// batch buffer sizes, showing how the bulk path is amortized as batches grow.
func BenchmarkScanner(b *testing.B) {
	const window = 64
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(i*131 + 7)
	}

	type bcase = struct {
		name string
		h    rollinghash.Hash
	}
	cases := make([]bcase, 0, len(allHashes)+1)
	for _, h := range allHashes {
		cases = append(cases, bcase{h.name, h.new()})
	}
	cases = append(cases, bcase{"fallback", noBulkRoller{adler32.New()}})
	bufSizes := []int{4 << 10, 64 << 10, 1 << 20}

	for _, c := range cases {
		for _, bs := range bufSizes {
			b.Run(fmt.Sprintf("%s/buf=%dKiB", c.name, bs>>10), func(b *testing.B) {
				s := rollinghash.NewScanner(nil, c.h, window)
				s.Buffer(make([]byte, bs))
				r := bytes.NewReader(data)

				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					r.Reset(data)
					s.Reset(r)
					for s.Scan() {
					}
					if s.Err() != nil {
						b.Fatal(s.Err())
					}
				}
			})
		}
	}
}
