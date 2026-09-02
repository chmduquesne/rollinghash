// Package bench compares cdc/compat/restic against the real
// github.com/restic/chunker: a parity test with the real package as the oracle,
// and a head-to-head throughput benchmark.
package bench

import (
	"bytes"
	"io"
	"testing"

	compat "github.com/chmduquesne/rollinghash/v4/cdc/compat/restic"
	rc "github.com/restic/chunker"
)

// resticDefaultPol is restic's historical default polynomial: a valid
// degree-53 irreducible polynomial, convenient as a fixed vector.
const resticDefaultPol = rc.Pol(0x3DA3358B4DC173)

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

// structData embeds a low-entropy run to vary the boundary distribution.
func structData(n int) []byte {
	b := randData(n)
	for i := n / 4; i < n/2 && i < len(b); i++ {
		b[i] = 0x5c
	}
	return b
}

func drainReal(tb testing.TB, c *rc.Chunker) []rc.Chunk {
	tb.Helper()
	var out []rc.Chunk
	for {
		chunk, err := c.Next(nil)
		if err == io.EOF {
			return out
		}
		if err != nil {
			tb.Fatalf("restic/chunker Next: %v", err)
		}
		out = append(out, chunk)
	}
}

func drainCompat(tb testing.TB, c *compat.Chunker) []compat.Chunk {
	tb.Helper()
	var out []compat.Chunk
	for {
		chunk, err := c.Next(nil)
		if err == io.EOF {
			return out
		}
		if err != nil {
			tb.Fatalf("compat Next: %v", err)
		}
		out = append(out, chunk)
	}
}

// TestParityRestic is the byte-identical proof: for a grid of polynomials,
// bounds, average-bits and data shapes, cdc/compat/restic must produce the same
// Start/Length/Data as the real github.com/restic/chunker for every chunk, and
// the same Cut for every chunk except a final chunk shorter than MinSize, where
// restic reports its (by its own admission meaningless) uninitialised rolling
// digest while compat reports the real trailing-window fingerprint — see the
// documented difference.
func TestParityRestic(t *testing.T) {
	pols := []rc.Pol{resticDefaultPol}
	if p, err := rc.RandomPolynomial(); err == nil {
		pols = append(pols, p)
	}

	shapes := map[string]func(int) []byte{"rand": randData, "struct": structData}
	sizes := []int{0, 5, 40, 64, 200, 4096, 300_000, 1 << 20}
	configs := []struct {
		min, max uint
		bits     int
	}{
		{512, 4096, 12},
		{1024, 65536, 14},
		{64, 512, 8},
		{2048, 32768, 13},
	}

	for _, pol := range pols {
		for shapeName, gen := range shapes {
			for _, n := range sizes {
				data := gen(n)
				for _, cfg := range configs {
					real := rc.New(bytes.NewReader(data), pol,
						rc.WithBoundaries(cfg.min, cfg.max), rc.WithAverageBits(cfg.bits))
					want := drainReal(t, real)

					got := drainCompat(t, compat.New(bytes.NewReader(data), compat.Pol(pol),
						compat.WithBoundaries(cfg.min, cfg.max), compat.WithAverageBits(cfg.bits)))

					if len(got) != len(want) {
						t.Fatalf("pol=%#x %s n=%d %v: %d chunks, want %d",
							uint64(pol), shapeName, n, cfg, len(got), len(want))
					}
					for i := range want {
						w, g := want[i], got[i]
						if g.Start != w.Start || g.Length != w.Length || !bytes.Equal(g.Data, w.Data) {
							t.Fatalf("pol=%#x %s n=%d %v chunk %d: {%d,%d} != {%d,%d}",
								uint64(pol), shapeName, n, cfg, i, g.Start, g.Length, w.Start, w.Length)
						}
						// restic's Cut on a final chunk shorter than MinSize is its
						// uninitialised rolling digest, not a window fingerprint
						// (its source calls the value meaningless); compat reports
						// the real fingerprint of the last 64 bytes, or 0 when the
						// whole stream is shorter than 64. Boundaries still match
						// exactly; skip only this field here.
						if i == len(want)-1 && w.Length < cfg.min {
							if n < 64 && g.Cut != 0 {
								t.Errorf("pol=%#x %s n=%d %v: sub-window final Cut = %#x, want 0",
									uint64(pol), shapeName, n, cfg, g.Cut)
							}
							continue
						}
						if g.Cut != w.Cut {
							t.Fatalf("pol=%#x %s n=%d %v chunk %d (len %d): Cut %#x != %#x",
								uint64(pol), shapeName, n, cfg, i, g.Length, g.Cut, w.Cut)
						}
					}
				}
			}
		}
	}
}

func benchData() []byte { return randData(8 << 20) }

func BenchmarkRestic_Real(b *testing.B) {
	data := benchData()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		c := rc.New(bytes.NewReader(data), resticDefaultPol)
		for {
			if _, err := c.Next(nil); err == io.EOF {
				break
			} else if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkRestic_Compat(b *testing.B) {
	data := benchData()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for range b.N {
		c := compat.New(bytes.NewReader(data), compat.Pol(resticDefaultPol))
		for {
			if _, err := c.Next(nil); err == io.EOF {
				break
			} else if err != nil {
				b.Fatal(err)
			}
		}
	}
}
