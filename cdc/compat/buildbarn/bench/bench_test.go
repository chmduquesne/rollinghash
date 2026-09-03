// Package bench compares cdc/compat/buildbarn against the real
// github.com/buildbarn/go-cdc: a byte-for-byte parity test with the real package
// as the oracle (proving the compat layer — and the cdc/* ports it wraps —
// reproduce buildbarn exactly, including cdc/fastcdc's WithInclusiveBoundary and
// cdc/repmaxsfxcdc's WithSubstitutionBox), plus a head-to-head throughput
// benchmark.
package bench

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	realcdc "github.com/buildbarn/go-cdc"
	compat "github.com/chmduquesne/rollinghash/v4/cdc/compat/buildbarn"
)

const bufSize = 128 << 10

func randData(n int) []byte {
	d := make([]byte, n)
	var x uint64 = 0x9e3779b97f4a7c15
	for i := range d {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		d[i] = byte(x)
	}
	return d
}

// sortedData is monotone runs — the worst case the substitution box exists to
// fix, and a good stress for every algorithm's forced-cut paths.
func sortedData(n int) []byte {
	d := make([]byte, n)
	var v byte
	var x uint64 = 0x2545f4914f6cdd1d
	run := 0
	for i := range d {
		if run == 0 {
			x ^= x << 13
			x ^= x >> 7
			x ^= x << 17
			run = 1 + int(x&255)
		}
		d[i] = v
		v++
		run--
	}
	return d
}

func lowEntropy(n int) []byte {
	d := make([]byte, n)
	var x uint64 = 0x243f6a8885a308d3
	for i := range d {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		d[i] = 0x20 + byte(x&0x3f)
	}
	return d
}

type chunkReader interface {
	ReadNextChunk() ([]byte, error)
}

func drain(tb testing.TB, r chunkReader) [][]byte {
	tb.Helper()
	var out [][]byte
	for {
		b, err := r.ReadNextChunk()
		if err == io.EOF {
			return out
		}
		if err != nil {
			tb.Fatalf("ReadNextChunk: %v", err)
		}
		out = append(out, append([]byte(nil), b...))
	}
}

func reader(data []byte) *bufio.Reader {
	return bufio.NewReaderSize(bytes.NewReader(data), bufSize)
}

var (
	realGT = &realcdc.FastContentDefinedChunkerGearTable
	ourGT  = &compat.FastContentDefinedChunkerGearTable
)

// algo pairs a real buildbarn chunker with the compat one for the same config.
type algo struct {
	name string
	peek int // expected GetMaximumPeekSizeBytes
	real func() realChunker
	ours func() oursChunker
}

type realChunker interface {
	NewChunkReader(realcdc.Peeker) realcdc.ChunkReader
	GetMaximumPeekSizeBytes() int
}
type oursChunker interface {
	NewChunkReader(compat.Peeker) compat.ChunkReader
	GetMaximumPeekSizeBytes() int
}

func algos() []algo {
	seed := []byte("parity-seed")
	realBox := realcdc.NewSeededSubstitutionBox(seed)
	ourBox := compat.NewSeededSubstitutionBox(seed)
	realSeededGT := realcdc.NewSeededGearTable(seed)
	ourSeededGT := compat.NewSeededGearTable(seed)

	return []algo{
		{"fast/512", 4 * 512,
			func() realChunker { return realcdc.NewFastContentDefinedChunker(realGT, 512) },
			func() oursChunker { return compat.NewFastContentDefinedChunker(ourGT, 512) }},
		{"fast/8192", 4 * 8192,
			func() realChunker { return realcdc.NewFastContentDefinedChunker(realGT, 8192) },
			func() oursChunker { return compat.NewFastContentDefinedChunker(ourGT, 8192) }},
		{"max/2k-8k", 2048 + 8192,
			func() realChunker { return realcdc.NewMaxContentDefinedChunker(realGT, 2048, 8192) },
			func() oursChunker { return compat.NewMaxContentDefinedChunker(ourGT, 2048, 8192) }},
		{"simplemax/2k-8k", 2048 + 8192,
			func() realChunker { return realcdc.NewSimpleMaxContentDefinedChunker(realGT, 2048, 8192) },
			func() oursChunker { return compat.NewSimpleMaxContentDefinedChunker(ourGT, 2048, 8192) }},
		{"repmax/4k-8k", 2*4096 + 8192,
			func() realChunker { return realcdc.NewRepMaxContentDefinedChunker(realGT, 4096, 8192) },
			func() oursChunker { return compat.NewRepMaxContentDefinedChunker(ourGT, 4096, 8192) }},
		{"repmax/seededgt", 2*4096 + 8192,
			func() realChunker { return realcdc.NewRepMaxContentDefinedChunker(realSeededGT, 4096, 8192) },
			func() oursChunker { return compat.NewRepMaxContentDefinedChunker(ourSeededGT, 4096, 8192) }},
		{"repmaxsfx/identity", 2*4096 + 8192,
			func() realChunker {
				return realcdc.NewRepMaxSfxContentDefinedChunker(&realcdc.NoSubstitutionBox, 4096, 8192)
			},
			func() oursChunker {
				return compat.NewRepMaxSfxContentDefinedChunker(&compat.NoSubstitutionBox, 4096, 8192)
			}},
		{"repmaxsfx/simple", 2*4096 + 8192,
			func() realChunker { return realcdc.NewSimpleRepMaxSfxContentDefinedChunker(4096, 8192) },
			func() oursChunker { return compat.NewSimpleRepMaxSfxContentDefinedChunker(4096, 8192) }},
		{"repmaxsfx/seededbox", 2*4096 + 8192,
			func() realChunker { return realcdc.NewRepMaxSfxContentDefinedChunker(realBox, 4096, 8192) },
			func() oursChunker { return compat.NewRepMaxSfxContentDefinedChunker(ourBox, 4096, 8192) }},
		{"ae/4k-16k", 16384,
			func() realChunker { return realcdc.NewAsymmetricExtremumContentDefinedChunker(4096, 16384) },
			func() oursChunker { return compat.NewAsymmetricExtremumContentDefinedChunker(4096, 16384) }},
		{"simpleae/4k-16k", 16384,
			func() realChunker { return realcdc.NewSimpleAsymmetricExtremumContentDefinedChunker(4096, 16384) },
			func() oursChunker { return compat.NewSimpleAsymmetricExtremumContentDefinedChunker(4096, 16384) }},
	}
}

func TestParityWithRealBuildbarn(t *testing.T) {
	datasets := map[string]func(int) []byte{
		"rand":   randData,
		"sorted": sortedData,
		"lowent": lowEntropy,
		"zeros":  func(n int) []byte { return make([]byte, n) },
	}
	sizes := []int{0, 1, 100, 4096, 8193, 200 * 1024, 1 << 20}

	for _, a := range algos() {
		if got := a.ours().GetMaximumPeekSizeBytes(); got != a.peek || got != a.real().GetMaximumPeekSizeBytes() {
			t.Errorf("%s: GetMaximumPeekSizeBytes ours=%d real=%d want=%d",
				a.name, got, a.real().GetMaximumPeekSizeBytes(), a.peek)
		}
		for dname, gen := range datasets {
			for _, n := range sizes {
				data := gen(n)
				want := drain(t, a.real().NewChunkReader(reader(data)))
				got := drain(t, a.ours().NewChunkReader(reader(data)))
				if !equalChunks(got, want) {
					t.Fatalf("%s/%s/%d: %d chunks vs %d (lens got=%v want=%v)",
						a.name, dname, n, len(got), len(want), lensOf(got), lensOf(want))
				}
			}
		}
	}
}

func TestSeededTablesMatchBuildbarn(t *testing.T) {
	// Same seed must yield the same chunk boundaries through both packages'
	// seeded-table derivations.
	seed := []byte("cross-check")
	data := randData(300 * 1024)

	realGear := realcdc.NewMaxContentDefinedChunker(realcdc.NewSeededGearTable(seed), 4096, 16384)
	ourGear := compat.NewMaxContentDefinedChunker(compat.NewSeededGearTable(seed), 4096, 16384)
	if !equalChunks(drain(t, ourGear.NewChunkReader(reader(data))), drain(t, realGear.NewChunkReader(reader(data)))) {
		t.Fatal("NewSeededGearTable: boundaries differ from buildbarn")
	}

	realSfx := realcdc.NewRepMaxSfxContentDefinedChunker(realcdc.NewSeededSubstitutionBox(seed), 4096, 8192)
	ourSfx := compat.NewRepMaxSfxContentDefinedChunker(compat.NewSeededSubstitutionBox(seed), 4096, 8192)
	if !equalChunks(drain(t, ourSfx.NewChunkReader(reader(data))), drain(t, realSfx.NewChunkReader(reader(data)))) {
		t.Fatal("NewSeededSubstitutionBox: boundaries differ from buildbarn")
	}
}

func equalChunks(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

func lensOf(c [][]byte) []int {
	out := make([]int, len(c))
	for i := range c {
		out[i] = len(c[i])
	}
	return out
}

func BenchmarkVsBuildbarn(b *testing.B) {
	data := randData(8 << 20)
	for _, a := range algos() {
		real := a.real()
		ours := a.ours()
		b.Run(a.name+"/buildbarn", func(b *testing.B) {
			benchDrain(b, data, func() chunkReader { return real.NewChunkReader(reader(data)) })
		})
		b.Run(a.name+"/ours", func(b *testing.B) {
			benchDrain(b, data, func() chunkReader { return ours.NewChunkReader(reader(data)) })
		})
	}
}

func benchDrain(b *testing.B, data []byte, mk func() chunkReader) {
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r := mk()
		for {
			_, err := r.ReadNextChunk()
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
