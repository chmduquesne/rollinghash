// Package bench compares this repo's CDC packages against the real
// github.com/PlakarKorp/go-cdc-chunkers: parity tests with real plakar as the
// oracle (cdc/compat/gocdc for the unkeyed families, cdc/fastcdc for keyed FastCDC)
// and a head-to-head throughput benchmark.
package bench

import (
	"bytes"
	"io"
	"testing"

	chunkers "github.com/PlakarKorp/go-cdc-chunkers"
	_ "github.com/PlakarKorp/go-cdc-chunkers/chunkers/fastcdc"
	_ "github.com/PlakarKorp/go-cdc-chunkers/chunkers/jc"
	_ "github.com/PlakarKorp/go-cdc-chunkers/chunkers/ultracdc"
	gocdc "github.com/chmduquesne/rollinghash/v4/cdc/compat/gocdc"
)

func randData(n int) []byte {
	data := make([]byte, n)
	var x uint64 = 0x9e3779b97f4a7c15
	for i := range data {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		data[i] = byte(x)
	}
	return data
}

// structData embeds a low-entropy run to exercise UltraCDC's identical-window
// path.
func structData(n int) []byte {
	d := randData(n)
	for i := n / 4; i < n/2 && i < len(d); i++ {
		d[i] = 0x5c
	}
	return d
}

type opts struct{ min, normal, max int }

func plakarOpts(o opts) *chunkers.ChunkerOpts {
	return &chunkers.ChunkerOpts{MinSize: o.min, MaxSize: o.max, NormalSize: o.normal}
}
func gocdcOpts(o opts) *gocdc.ChunkerOpts {
	return &gocdc.ChunkerOpts{MinSize: o.min, MaxSize: o.max, NormalSize: o.normal}
}

func drainPlakar(tb testing.TB, c *chunkers.Chunker) [][]byte {
	tb.Helper()
	var out [][]byte
	for {
		b, err := c.Next()
		if len(b) > 0 {
			out = append(out, append([]byte(nil), b...))
		}
		if err == io.EOF {
			return out
		}
		if err != nil {
			tb.Fatalf("plakar Next: %v", err)
		}
	}
}

func drainGocdc(tb testing.TB, c *gocdc.Chunker) [][]byte {
	tb.Helper()
	var out [][]byte
	for {
		b, err := c.Next()
		if len(b) > 0 {
			out = append(out, append([]byte(nil), b...))
		}
		if err == io.EOF {
			return out
		}
		if err != nil {
			tb.Fatalf("gocdc Next: %v", err)
		}
	}
}

var algorithms = []string{
	"fastcdc", "fastcdc-v1.0.0",
	"jc", "jc-v1.0.0", "jc-v1.1.0",
	"ultracdc", "ultracdc-v1.0.0",
}

// TestParityWithRealPlakar diffs gocdc's output against real plakar,
// byte-for-byte, for every algorithm/variant across a few configs and data
// shapes. This is a stronger check than the in-repo ports: it also proves those
// ports are faithful.
func TestParityWithRealPlakar(t *testing.T) {
	configs := map[string]opts{
		"default":  {2 * 1024, 8 * 1024, 64 * 1024},
		"ultradef": {2 * 1024, 10 * 1024, 64 * 1024},
		"small":    {512, 4 * 1024, 32 * 1024},
		"tiny":     {128, 1024, 8 * 1024},
	}
	datasets := map[string][]byte{
		"rand512k":   randData(512 * 1024),
		"struct256k": structData(256 * 1024),
		"rand2000":   randData(2000),
		"tail":       randData(70*1024 + 123),
	}

	for _, algo := range algorithms {
		for cname, cfg := range configs {
			// UltraCDC's default NormalSize is 10 KiB, not a power of two;
			// FastCDC/JC want the 8 KiB default. Skip the mismatched pairing.
			if algo[:2] == "ul" && cname == "default" {
				continue
			}
			if algo[:2] != "ul" && cname == "ultradef" {
				continue
			}
			for dname, data := range datasets {
				pc, err := chunkers.NewChunker(algo, bytes.NewReader(data), plakarOpts(cfg))
				if err != nil {
					t.Fatalf("%s/%s: plakar NewChunker: %v", algo, cname, err)
				}
				want := drainPlakar(t, pc)

				gc, err := gocdc.NewChunker(algo, bytes.NewReader(data), gocdcOpts(cfg))
				if err != nil {
					t.Fatalf("%s/%s: gocdc NewChunker: %v", algo, cname, err)
				}
				got := drainGocdc(t, gc)

				if len(got) != len(want) {
					t.Fatalf("%s/%s/%s: got %d chunks, want %d", algo, cname, dname, len(got), len(want))
				}
				for i := range want {
					if !bytes.Equal(got[i], want[i]) {
						t.Fatalf("%s/%s/%s chunk %d: %d bytes vs %d bytes",
							algo, cname, dname, i, len(got[i]), len(want[i]))
					}
				}
			}
		}
	}
}

// BenchmarkVsPlakar/<algo>/{ours,plakar} chunks the same 1 MiB buffer with both
// implementations, reusing one chunker via Reset per iteration.
func BenchmarkVsPlakar(b *testing.B) {
	data := randData(1 << 20)
	bench := []struct {
		name string
		cfg  opts
	}{
		{"fastcdc", opts{2 * 1024, 8 * 1024, 64 * 1024}},
		{"jc", opts{2 * 1024, 8 * 1024, 64 * 1024}},
		{"ultracdc", opts{2 * 1024, 10 * 1024, 64 * 1024}},
	}

	for _, tc := range bench {
		b.Run(tc.name+"/ours", func(b *testing.B) {
			c, err := gocdc.NewChunker(tc.name, bytes.NewReader(data), gocdcOpts(tc.cfg))
			if err != nil {
				b.Fatal(err)
			}
			r := bytes.NewReader(data)
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				r.Reset(data)
				c.Reset(r)
				for {
					_, err := c.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
		b.Run(tc.name+"/plakar", func(b *testing.B) {
			c, err := chunkers.NewChunker(tc.name, bytes.NewReader(data), plakarOpts(tc.cfg))
			if err != nil {
				b.Fatal(err)
			}
			r := bytes.NewReader(data)
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				r.Reset(data)
				c.Reset(r)
				for {
					_, err := c.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
