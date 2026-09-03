package cdc_test

import (
	"bytes"
	"io"
	"testing"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/buzhash64"
	"github.com/chmduquesne/rollinghash/v4/cdc/aecdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/fastcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/jumpchunker"
	"github.com/chmduquesne/rollinghash/v4/cdc/maxcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/repmaxcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/repmaxsfxcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/ultracdc"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
)

const h2hSize = 8 << 20

func h2hRandom(n int) []byte {
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

// h2hLowEntropy is a 64-symbol alphabet, independent per byte: ~6 bits/byte,
// no long runs. Lower-entropy but non-degenerate input — every algorithm still
// chunks normally, but the suffix chunker sees more first-byte ties and the
// Gear masks fire a little less cleanly.
func h2hLowEntropy(n int) []byte {
	data := make([]byte, n)
	var x uint64 = 0x243f6a8885a308d3
	for i := range data {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		data[i] = 0x20 + byte(x&0x3f)
	}
	return data
}

func h2hDatasets() []struct {
	name string
	data []byte
} {
	return []struct {
		name string
		data []byte
	}{
		{"random", h2hRandom(h2hSize)},
		{"lowentropy", h2hLowEntropy(h2hSize)},
	}
}

// h2hAlgos lists every CDC algorithm, each tuned for a ~8 KiB average chunk.
func h2hAlgos() []struct {
	name string
	make func(io.Reader) rollinghash.Chunker
} {
	return []struct {
		name string
		make func(io.Reader) rollinghash.Chunker
	}{
		{"parent-gear", func(r io.Reader) rollinghash.Chunker {
			return rollinghash.NewChunker(r, gearhash64.New(), 56, 0x1fff, rollinghash.WithBoundaries(2<<10, 64<<10))
		}},
		{"parent-buzhash", func(r io.Reader) rollinghash.Chunker {
			return rollinghash.NewChunker(r, buzhash64.New(), 56, 0x1fff, rollinghash.WithBoundaries(2<<10, 64<<10))
		}},
		{"fastcdc", func(r io.Reader) rollinghash.Chunker {
			return fastcdc.New(r, gearhash64.New(), 2<<10, 8<<10, 64<<10)
		}},
		{"jumpchunker", func(r io.Reader) rollinghash.Chunker {
			return jumpchunker.New(r, gearhash64.New(), 8<<10, 2<<10, 64<<10)
		}},
		{"ultracdc", func(r io.Reader) rollinghash.Chunker {
			return ultracdc.New(r, 2<<10, 8<<10, 64<<10)
		}},
		{"maxcdc", func(r io.Reader) rollinghash.Chunker {
			return maxcdc.New(r, gearhash64.New(), 4<<10, 13<<10)
		}},
		{"repmaxcdc", func(r io.Reader) rollinghash.Chunker {
			return repmaxcdc.New(r, gearhash64.New(), 6<<10, 8<<10)
		}},
		{"repmaxsfxcdc", func(r io.Reader) rollinghash.Chunker {
			return repmaxsfxcdc.New(r, 6<<10, 8<<10)
		}},
		{"aecdc", func(r io.Reader) rollinghash.Chunker {
			return aecdc.New(r, 7680, 64<<10)
		}},
	}
}

// TestHeadToHeadChunkStats prints the chunk-size distribution each algorithm
// produces on each dataset, so BenchmarkHeadToHead's throughput can be read
// against comparable work. Run with: go test -run TestHeadToHeadChunkStats -v ./cdc/
func TestHeadToHeadChunkStats(t *testing.T) {
	for _, ds := range h2hDatasets() {
		for _, a := range h2hAlgos() {
			c := a.make(bytes.NewReader(ds.data))
			n, total, mx, mn := 0, 0, 0, 1<<62
			for c.Next() {
				l := len(c.Bytes())
				n++
				total += l
				if l > mx {
					mx = l
				}
				if l < mn {
					mn = l
				}
			}
			if err := c.Err(); err != nil {
				t.Fatal(err)
			}
			if total != len(ds.data) {
				t.Fatalf("%s/%s: reassembled %d of %d bytes", ds.name, a.name, total, len(ds.data))
			}
			t.Logf("%-8s %-14s  chunks=%5d  avg=%5d  min=%5d  max=%6d", ds.name, a.name, n, total/n, mn, mx)
		}
	}
}

// BenchmarkHeadToHead runs every CDC algorithm over the same buffers. Run with:
//
//	go test -run '^$' -bench BenchmarkHeadToHead -benchmem ./cdc/
//
// then TestHeadToHeadChunkStats for the chunk sizes each produced.
func BenchmarkHeadToHead(b *testing.B) {
	for _, ds := range h2hDatasets() {
		for _, a := range h2hAlgos() {
			b.Run(ds.name+"/"+a.name, func(b *testing.B) {
				c := a.make(bytes.NewReader(ds.data))
				r := bytes.NewReader(ds.data)
				b.SetBytes(int64(len(ds.data)))
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					r.Reset(ds.data)
					c.Reset(r)
					for c.Next() {
						_ = c.Bytes()
					}
					if c.Err() != nil {
						b.Fatal(c.Err())
					}
				}
			})
		}
	}
}
