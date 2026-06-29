package rollinghash_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"testing/iotest"

	"github.com/chmduquesne/rollinghash/v4"
)

// collectBatchRollerSums runs a BatchRoller to exhaustion and returns the
// concatenation of every batch's Sums(), checking the per-batch alignment
// invariant (len(Sums) == len(Bytes)-window+1) along the way.
func collectBatchRollerSums(t *testing.T, name string, s rollinghash.BatchRoller, window int) []uint64 {
	t.Helper()
	var got []uint64
	for s.Next() {
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

// TestBatchRollerWindowSize verifies that WindowSize() returns the window
// passed to NewBatchRoller, independent of stream state.
func TestBatchRollerWindowSize(t *testing.T) {
	const window = 56
	s := rollinghash.NewBatchRoller(bytes.NewReader(nil), allHashes[0].new(), window)
	if s.WindowSize() != window {
		t.Fatalf("WindowSize() = %d, want %d", s.WindowSize(), window)
	}
}

// TestBatchRoller checks that the concatenated Sums() over a multi-batch
// stream equal the classic hash of every window position.
func TestBatchRoller(t *testing.T) {
	// Larger than the default 64 KiB buffer, so several batches are produced.
	data := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 5000)
	const window = 56
	for _, h := range allHashes {
		want := batchRollOracle(h.new(), data, window)
		s := rollinghash.NewBatchRoller(bytes.NewReader(data), h.new(), window)
		got := collectBatchRollerSums(t, h.name, s, window)
		equalSums(t, h.name, got, want)
	}
}

// TestBatchRollerBatchBoundaries is the load-bearing test: a window that
// straddles a Next() boundary must produce the same checksum as if the input
// had never been split. It feeds the same input through adversarial buffer
// sizes (and a one-byte-at-a-time reader to stress the refill loop) and
// compares against a single oracle.
func TestBatchRollerBatchBoundaries(t *testing.T) {
	data := make([]byte, 500)
	for i := range data {
		data[i] = byte(i*73 + 11)
	}
	const window = 16
	bufSizes := []int{window, window + 1, window + 7, 2 * window, 3*window - 1, 97, len(data)}

	for _, h := range allHashes {
		want := batchRollOracle(h.new(), data, window)
		for _, bs := range bufSizes {
			// Plain reader.
			s := rollinghash.NewBatchRoller(bytes.NewReader(data), h.new(), window, rollinghash.WithBuffer(make([]byte, bs)))
			got := collectBatchRollerSums(t, h.name, s, window)
			equalSums(t, h.name, got, want)

			// One byte per Read, to exercise the buffer-fill loop.
			s = rollinghash.NewBatchRoller(iotest.OneByteReader(bytes.NewReader(data)), h.new(), window, rollinghash.WithBuffer(make([]byte, bs)))
			got = collectBatchRollerSums(t, h.name, s, window)
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

// TestBatchRollerBytes verifies that each batch's Bytes() holds the exact
// source bytes for that batch - across batch boundaries and adversarial buffer
// sizes. (Sums()-only tests miss a buffer that corrupts the bytes it hands back.)
func TestBatchRollerBytes(t *testing.T) {
	data := testData(5000)
	const window = 16
	for _, bs := range []int{window, window + 1, 64, 333, len(data)} {
		s := rollinghash.NewBatchRoller(bytes.NewReader(data), allHashes[0].new(), window, rollinghash.WithBuffer(make([]byte, bs)))
		off := 0
		for s.Next() {
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

// TestBatchRollerShortInput verifies that an input shorter than window yields
// no batches and zero-value accessors, while exactly window bytes yields one
// sum.
func TestBatchRollerShortInput(t *testing.T) {
	const window = 16
	for _, h := range allHashes {
		for _, n := range []int{0, 1, window - 1} {
			s := rollinghash.NewBatchRoller(bytes.NewReader(make([]byte, n)), h.new(), window)
			if s.Next() {
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
		s := rollinghash.NewBatchRoller(bytes.NewReader(make([]byte, window)), h.new(), window)
		got := collectBatchRollerSums(t, h.name, s, window)
		if len(got) != 1 {
			t.Errorf("[%s] exactly window: got %d sums, want 1", h.name, len(got))
		}
	}
}

// TestBatchRollerAccessorLifecycle verifies that Sums() and Bytes() are nil
// before the first Next() call and after Next() returns false, even on a
// stream that produces batches.
func TestBatchRollerAccessorLifecycle(t *testing.T) {
	const window = 16
	data := testData(200)
	for _, h := range allHashes {
		s := rollinghash.NewBatchRoller(bytes.NewReader(data), h.new(), window)

		if s.Sums() != nil || s.Bytes() != nil {
			t.Errorf("[%s] expected nil Sums/Bytes before first Next", h.name)
		}

		for s.Next() {
		}
		if err := s.Err(); err != nil {
			t.Fatalf("[%s] Err: %v", h.name, err)
		}

		if s.Sums() != nil || s.Bytes() != nil {
			t.Errorf("[%s] expected nil Sums/Bytes after Next returns false", h.name)
		}

		// Calling Next() again after exhaustion must also return nil.
		if s.Next() {
			t.Errorf("[%s] Next() returned true after exhaustion", h.name)
		}
		if s.Sums() != nil || s.Bytes() != nil {
			t.Errorf("[%s] expected nil Sums/Bytes on repeated Next after exhaustion", h.name)
		}
	}
}

// TestBatchRollerError verifies that a reader error is surfaced through Err
// and stops the roll.
func TestBatchRollerError(t *testing.T) {
	boom := errors.New("boom")
	for _, h := range allHashes {
		s := rollinghash.NewBatchRoller(iotest.ErrReader(boom), h.new(), 16)
		if s.Next() {
			t.Errorf("[%s] expected Next to fail on reader error", h.name)
		}
		if !errors.Is(s.Err(), boom) {
			t.Errorf("[%s] expected Err to be boom, got %v", h.name, s.Err())
		}
	}
}

// FuzzBatchRoller cross-checks the BatchRoller against the oracle on random
// data, window sizes, and buffer sizes, using both the bulk fast path and the
// Write+Roll fallback. It verifies three invariants per batch:
//   - alignment: len(Sums()) == len(Bytes()) - window + 1
//   - bytes: Bytes() matches the corresponding slice of the original input
//   - sums: concatenated Sums() equals the classic per-window hash (the oracle)
func FuzzBatchRoller(f *testing.F) {
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

		for _, hc := range allHashes {
			want := batchRollOracle(hc.new(), data, window)
			h := hc.new()

			s := rollinghash.NewBatchRoller(bytes.NewReader(data), h, window, rollinghash.WithBuffer(make([]byte, bufSize)))

			var got []uint64
			off := 0
			for s.Next() {
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

// BenchmarkBatchRoller measures steady-state throughput, reusing one
// BatchRoller (and its buffers) across iterations via Reset so the numbers
// reflect rolling, not per-stream setup. It covers every bulk fast path
// implementation (bozo64, buzhash64, gearhash64, rabinkarp64) plus the
// Write+Roll fallback, across batch buffer sizes, showing how the bulk path
// is amortized as batches grow.
func BenchmarkBatchRoller(b *testing.B) {
	const window = 56
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(i*131 + 7)
	}

	type bcase = struct {
		name string
		h    rollinghash.Hash
	}
	cases := make([]bcase, 0, len(allHashes))
	for _, h := range allHashes {
		cases = append(cases, bcase{h.name, h.new()})
	}
	bufSizes := []int{4 << 10, 64 << 10, 1 << 20}

	for _, c := range cases {
		for _, bs := range bufSizes {
			b.Run(fmt.Sprintf("%s/buf=%dKiB", c.name, bs>>10), func(b *testing.B) {
				s := rollinghash.NewBatchRoller(nil, c.h, window, rollinghash.WithBuffer(make([]byte, bs)))
				r := bytes.NewReader(data)

				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					r.Reset(data)
					s.Reset(r)
					for s.Next() {
					}
					if s.Err() != nil {
						b.Fatal(s.Err())
					}
				}
			})
		}
	}
}
