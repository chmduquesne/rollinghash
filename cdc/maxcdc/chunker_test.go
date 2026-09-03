package maxcdc_test

import (
	"bytes"
	"errors"
	"testing"
	"testing/iotest"

	"github.com/chmduquesne/rollinghash/v4/buzhash64"
	"github.com/chmduquesne/rollinghash/v4/cdc/maxcdc"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
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

func structData(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i*2654435761 + i/7)
	}
	return data
}

func collect(t *testing.T, c *maxcdc.Chunker) [][]byte {
	t.Helper()
	var out [][]byte
	for c.Next() {
		out = append(out, append([]byte(nil), c.Bytes()...))
	}
	if err := c.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	return out
}

// refMax is an independent whole-buffer transcription of buildbarn/go-cdc's
// simpleMaxChunkReader.ReadNextChunk (v0.0.10), used as the oracle. buildbarn
// ships NewSimpleMaxContentDefinedChunker precisely as a reference for
// NewMaxContentDefinedChunker; we port the simple one here rather than pull the
// package as a nested bench module because its go.mod requires Go 1.26. Keep
// this in sync with upstream if the algorithm changes.
func refMax(data []byte, gear [256]uint64, min, max int) [][]byte {
	peek := min + max
	var out [][]byte
	for pos := 0; pos < len(data); {
		d := data[pos:]
		if len(d) > peek {
			d = d[:peek]
		}
		if len(d) < 2*min {
			out = append(out, d)
			break
		}
		d = d[:len(d)-min]

		var hash uint64
		for _, b := range d[min-64 : min] {
			hash = (hash << 1) + gear[b]
		}
		bestHash, best := hash, 0
		for i, b := range d[min:] {
			hash = (hash << 1) + gear[b]
			if bestHash < hash {
				bestHash, best = hash, i+1
			}
		}
		n := min + best
		out = append(out, data[pos:pos+n])
		pos += n
	}
	return out
}

func sameChunks(a, b [][]byte) bool {
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

func lens(chunks [][]byte) []int {
	out := make([]int, len(chunks))
	for i, c := range chunks {
		out[i] = len(c)
	}
	return out
}

func TestChunkerMatchesReference(t *testing.T) {
	configs := []struct{ min, max int }{
		{256, 512},
		{256, 1024},
		{512, 4 * 1024},
		{1024, 8 * 1024},
		{64, 4096},
	}
	datasets := map[string][]byte{
		"rand300k":   randData(300 * 1024),
		"struct200k": structData(200 * 1024),
		"rand1k":     randData(1024),
		"tail":       randData(40*1024 + 777),
		"tiny":       randData(40),
		"empty":      nil,
	}
	table := gearhash64.New().Table()

	for _, cfg := range configs {
		for name, data := range datasets {
			want := refMax(data, table, cfg.min, cfg.max)
			got := collect(t, maxcdc.New(bytes.NewReader(data), gearhash64.New(), cfg.min, cfg.max))

			if !sameChunks(got, want) {
				t.Fatalf("min=%d max=%d %s: %d chunks, want %d (lens got=%v want=%v)",
					cfg.min, cfg.max, name, len(got), len(want), lens(got), lens(want))
			}
			if joined := bytes.Join(got, nil); !bytes.Equal(joined, data) {
				t.Fatalf("min=%d max=%d %s: reassembled %d bytes, want %d", cfg.min, cfg.max, name, len(joined), len(data))
			}
			for i, ch := range got {
				last := i == len(got)-1
				if !last && (len(ch) < cfg.min || len(ch) > cfg.max) {
					t.Fatalf("min=%d max=%d %s: chunk %d has %d bytes, want [%d, %d]",
						cfg.min, cfg.max, name, i, len(ch), cfg.min, cfg.max)
				}
				if last && len(data) >= cfg.min && len(ch) > cfg.max {
					t.Fatalf("min=%d max=%d %s: final chunk %d bytes exceeds max", cfg.min, cfg.max, name, len(ch))
				}
			}
		}
	}
}

func TestChunkerOneByteReader(t *testing.T) {
	data := randData(120 * 1024)
	want := collect(t, maxcdc.New(bytes.NewReader(data), gearhash64.New(), 512, 4*1024))
	got := collect(t, maxcdc.New(iotest.OneByteReader(bytes.NewReader(data)), gearhash64.New(), 512, 4*1024))
	if !sameChunks(got, want) {
		t.Fatalf("one-byte reader: got %d chunks, want %d", len(got), len(want))
	}
}

func TestChunkerReset(t *testing.T) {
	a := randData(80 * 1024)
	b := structData(90 * 1024)
	c := maxcdc.New(bytes.NewReader(a), gearhash64.New(), 512, 4*1024)
	got1 := collect(t, c)
	c.Reset(bytes.NewReader(b))
	got2 := collect(t, c)

	if !sameChunks(got1, collect(t, maxcdc.New(bytes.NewReader(a), gearhash64.New(), 512, 4*1024))) {
		t.Fatal("first pass changed under Reset")
	}
	if !sameChunks(got2, collect(t, maxcdc.New(bytes.NewReader(b), gearhash64.New(), 512, 4*1024))) {
		t.Fatal("post-Reset pass differs from a fresh chunker")
	}
}

func TestChunkerReaderError(t *testing.T) {
	boom := errors.New("boom")
	c := maxcdc.New(iotest.ErrReader(boom), gearhash64.New(), 256, 512)
	if c.Next() {
		t.Fatal("expected Next false on reader error")
	}
	if !errors.Is(c.Err(), boom) {
		t.Fatalf("Err = %v, want boom", c.Err())
	}
}

func TestChunkerSum(t *testing.T) {
	data := randData(200 * 1024)
	c := maxcdc.New(bytes.NewReader(data), gearhash64.New(), 512, 4*1024)
	g := gearhash64.New().Table()
	at := 0
	for c.Next() {
		at += len(c.Bytes())
		var want uint64
		for _, x := range data[at-64 : at] {
			want = (want << 1) + g[x]
		}
		if c.Sum() != want {
			t.Fatalf("chunk ending at %d: Sum %#x, want %#x", at, c.Sum(), want)
		}
	}
	if at != len(data) {
		t.Fatalf("reassembled %d of %d bytes", at, len(data))
	}
}

func BenchmarkChunker(b *testing.B) {
	data := randData(1 << 20)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	c := maxcdc.New(nil, gearhash64.New(), 8<<10, 64<<10)
	r := bytes.NewReader(data)
	for range b.N {
		r.Reset(data)
		c.Reset(r)
		for c.Next() {
			_ = c.Bytes()
		}
		if c.Err() != nil {
			b.Fatal(c.Err())
		}
	}
}

func TestNewPanics(t *testing.T) {
	mustPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Fatalf("%s: expected panic", name)
			}
		}()
		fn()
	}
	mustPanic("minSize too small", func() {
		maxcdc.New(bytes.NewReader(nil), gearhash64.New(), 32, 4096)
	})
	mustPanic("maxSize <= minSize", func() {
		maxcdc.New(bytes.NewReader(nil), gearhash64.New(), 256, 256)
	})
	mustPanic("hash without Table()", func() {
		maxcdc.New(bytes.NewReader(nil), buzhash64.New(), 256, 512)
	})
}
