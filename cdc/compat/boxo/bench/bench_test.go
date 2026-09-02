// Package bench compares cdc/compat/boxo against the real
// github.com/ipfs/boxo/chunker: a parity test with the real package as the
// oracle, and head-to-head throughput benchmarks.
package bench

import (
	"bytes"
	"io"
	"testing"

	compat "github.com/chmduquesne/rollinghash/v4/cdc/compat/boxo"
	chunk "github.com/ipfs/boxo/chunker"
)

func randData(n int) []byte {
	b := make([]byte, n)
	var x uint64 = 0x2545F4914F6CDD1D
	for i := range b {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		b[i] = byte(x)
	}
	return b
}

func structData(n int) []byte {
	b := randData(n)
	for i := n / 4; i < n/2 && i < len(b); i++ {
		b[i] = 0x5c
	}
	return b
}

func zeros(n int) []byte { return make([]byte, n) }

func drainCompat(tb testing.TB, s compat.Splitter) [][]byte {
	tb.Helper()
	var out [][]byte
	for {
		b, err := s.NextBytes()
		if err == io.EOF {
			return out
		}
		if err != nil {
			tb.Fatalf("compat NextBytes: %v", err)
		}
		out = append(out, append([]byte(nil), b...))
	}
}

func drainBoxo(tb testing.TB, s chunk.Splitter) [][]byte {
	tb.Helper()
	var out [][]byte
	for {
		b, err := s.NextBytes()
		if err == io.EOF {
			return out
		}
		if err != nil {
			tb.Fatalf("boxo NextBytes: %v", err)
		}
		out = append(out, append([]byte(nil), b...))
	}
}

func equalChunks(a, b [][]byte) (int, bool) {
	if len(a) != len(b) {
		n := len(a)
		if len(b) < n {
			n = len(b)
		}
		for i := 0; i < n; i++ {
			if !bytes.Equal(a[i], b[i]) {
				return i, false
			}
		}
		return n, false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return i, false
		}
	}
	return len(a), true
}

// TestParityBoxo is the byte-identical proof: for every built-in splitter over a
// grid of data shapes, sizes and parameters, cdc/compat/boxo must produce the
// exact same chunks as github.com/ipfs/boxo/chunker — which is what keeps the
// resulting CIDs identical.
func TestParityBoxo(t *testing.T) {
	shapes := map[string]func(int) []byte{"rand": randData, "struct": structData, "zeros": zeros}

	t.Run("rabin", func(t *testing.T) {
		sizes := []int{0, 15, 16, 17, 200, 4096, 200_000, 1 << 20, 5 << 20}
		bounds := []struct{ min, avg, max uint64 }{
			{2048 / 3 * 2, 2048, 3072}, {1365, 4096, 6144}, {21845, 65536, 98304},
			{87381, 262144, 393216}, {1000, 3000, 4500}, {17, 1000, 1500},
		}
		for name, gen := range shapes {
			for _, n := range sizes {
				data := gen(n)
				for _, b := range bounds {
					want := drainBoxo(t, chunk.NewRabinMinMax(bytes.NewReader(data), b.min, b.avg, b.max))
					got := drainCompat(t, compat.NewRabinMinMax(bytes.NewReader(data), b.min, b.avg, b.max))
					if i, ok := equalChunks(want, got); !ok {
						t.Fatalf("%s n=%d %v: diverge at chunk %d (%d vs %d chunks)",
							name, n, b, i, len(want), len(got))
					}
				}
				// NewRabin's derived bounds.
				for _, avg := range []uint64{1024, 4096, 262144} {
					want := drainBoxo(t, chunk.NewRabin(bytes.NewReader(data), avg))
					got := drainCompat(t, compat.NewRabin(bytes.NewReader(data), avg))
					if i, ok := equalChunks(want, got); !ok {
						t.Fatalf("%s n=%d NewRabin(%d): diverge at chunk %d", name, n, avg, i)
					}
				}
			}
		}
	})

	t.Run("buzhash", func(t *testing.T) {
		sizes := []int{0, 1, 31, 32, 33, 131071, 131072, 131073, 262143, 262144,
			524288, 524289, 700_000, 1 << 20, 3 << 20}
		for name, gen := range shapes {
			for _, n := range sizes {
				data := gen(n)
				want := drainBoxo(t, chunk.NewBuzhash(bytes.NewReader(data)))
				got := drainCompat(t, compat.NewBuzhash(bytes.NewReader(data)))
				if i, ok := equalChunks(want, got); !ok {
					t.Fatalf("%s n=%d: diverge at chunk %d (%d vs %d chunks)",
						name, n, i, len(want), len(got))
				}
			}
		}
	})

	t.Run("size", func(t *testing.T) {
		for _, n := range []int{0, 1, 1023, 1024, 1025, 200_000, 1 << 20} {
			data := randData(n)
			for _, sz := range []int64{1, 1024, 262144} {
				want := drainBoxo(t, chunk.NewSizeSplitter(bytes.NewReader(data), sz))
				got := drainCompat(t, compat.NewSizeSplitter(bytes.NewReader(data), sz))
				if i, ok := equalChunks(want, got); !ok {
					t.Fatalf("n=%d size=%d: diverge at chunk %d", n, sz, i)
				}
			}
		}
	})

	t.Run("FromString", func(t *testing.T) {
		data := randData(400_000)
		for _, spec := range []string{
			"", "default", "size-1024", "size-262144",
			"rabin", "rabin-4096", "rabin-65536",
			"rabin-min:512-avg:4096-max:16384", "rabin-min:1024-avg:8192-max:32768",
			"buzhash",
		} {
			ws, err := chunk.FromString(bytes.NewReader(data), spec)
			if err != nil {
				t.Fatalf("boxo FromString(%q): %v", spec, err)
			}
			cs, err := compat.FromString(bytes.NewReader(data), spec)
			if err != nil {
				t.Fatalf("compat FromString(%q): %v", spec, err)
			}
			if i, ok := equalChunks(drainBoxo(t, ws), drainCompat(t, cs)); !ok {
				t.Fatalf("FromString(%q): diverge at chunk %d", spec, i)
			}
		}
	})
}

func benchData() []byte { return randData(8 << 20) }

func benchmarkSplit(b *testing.B, mk func(io.Reader) interface {
	NextBytes() ([]byte, error)
}) {
	data := benchData()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		s := mk(bytes.NewReader(data))
		for {
			if _, err := s.NextBytes(); err == io.EOF {
				break
			} else if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkRabin_Boxo(b *testing.B) {
	benchmarkSplit(b, func(r io.Reader) interface{ NextBytes() ([]byte, error) } {
		return chunk.NewRabin(r, 262144)
	})
}
func BenchmarkRabin_Compat(b *testing.B) {
	benchmarkSplit(b, func(r io.Reader) interface{ NextBytes() ([]byte, error) } {
		return compat.NewRabin(r, 262144)
	})
}
func BenchmarkBuzhash_Boxo(b *testing.B) {
	benchmarkSplit(b, func(r io.Reader) interface{ NextBytes() ([]byte, error) } {
		return chunk.NewBuzhash(r)
	})
}
func BenchmarkBuzhash_Compat(b *testing.B) {
	benchmarkSplit(b, func(r io.Reader) interface{ NextBytes() ([]byte, error) } {
		return compat.NewBuzhash(r)
	})
}
