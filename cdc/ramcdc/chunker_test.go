package ramcdc_test

import (
	"bytes"
	"errors"
	"testing"
	"testing/iotest"

	"github.com/chmduquesne/rollinghash/v4/cdc/ramcdc"
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

func lowEntropy(n int) []byte {
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

func collect(t *testing.T, c *ramcdc.Chunker) [][]byte {
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

// refRAM is an independent whole-buffer transcription of dedup-bench's
// RAM_Chunking::find_cutpoint (native branch), used as the oracle. Keep this in
// sync with upstream if the algorithm changes.
func refRAM(data []byte, w, max int) [][]byte {
	var out [][]byte
	for pos := 0; pos < len(data); {
		d := data[pos:]
		if len(d) > max {
			d = d[:max]
		}
		n := len(d)
		cut := n
		if n > w {
			var mx byte
			for i := range w {
				if d[i] >= mx {
					mx = d[i]
				}
			}
			for i := w; i < n; i++ {
				if d[i] >= mx {
					cut = i
					break
				}
			}
		}
		out = append(out, data[pos:pos+cut])
		pos += cut
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
	configs := []struct{ w, max int }{
		{64, 256},
		{256, 1024},
		{512, 4 * 1024},
		{2 * 1024, 16 * 1024},
		{1, 64},
		{100, 100},
	}
	datasets := map[string][]byte{
		"rand300k":   randData(300 * 1024),
		"struct200k": structData(200 * 1024),
		"lowent128k": lowEntropy(128 * 1024),
		"rand1k":     randData(1024),
		"tail":       randData(40*1024 + 777),
		"tiny":       randData(9),
		"one":        randData(1),
		"empty":      nil,
	}

	for _, cfg := range configs {
		for name, data := range datasets {
			want := refRAM(data, cfg.w, cfg.max)
			got := collect(t, ramcdc.New(bytes.NewReader(data), cfg.w, cfg.max))
			if !sameChunks(got, want) {
				t.Fatalf("w=%d max=%d %s: %d chunks, want %d (got=%v want=%v)",
					cfg.w, cfg.max, name, len(got), len(want), lens(got), lens(want))
			}
			if joined := bytes.Join(got, nil); !bytes.Equal(joined, data) {
				t.Fatalf("w=%d max=%d %s: reassembled %d bytes, want %d", cfg.w, cfg.max, name, len(joined), len(data))
			}
			for i, ch := range got {
				if len(ch) > cfg.max {
					t.Fatalf("w=%d max=%d %s: chunk %d has %d bytes > max", cfg.w, cfg.max, name, i, len(ch))
				}
			}
		}
	}
}

func TestChunkerContentDefinedFlag(t *testing.T) {
	data := randData(200 * 1024)
	w, max := 512, 4*1024
	c := ramcdc.New(bytes.NewReader(data), w, max)
	for c.Next() {
		n := len(c.Bytes())
		if c.ContentDefined() {
			if n < w || n > max {
				t.Fatalf("content-defined chunk of %d bytes, want [%d, %d]", n, w, max)
			}
		} else if n != max && c.Offset()+n != len(data) {
			t.Fatalf("forced chunk of %d bytes that is neither maxSize nor the stream tail", n)
		}
	}
	if err := c.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestChunkerSum(t *testing.T) {
	data := randData(200 * 1024)
	w := 512
	c := ramcdc.New(bytes.NewReader(data), w, 4*1024)
	at := 0
	for c.Next() {
		ch := c.Bytes()
		at += len(ch)
		var want uint64
		if c.ContentDefined() {
			for _, b := range ch[:w] {
				if uint64(b) > want {
					want = uint64(b)
				}
			}
		} else {
			want = uint64(ch[len(ch)-1])
		}
		if c.Sum() != want {
			t.Fatalf("chunk ending at %d (cd=%v): Sum %d, want %d", at, c.ContentDefined(), c.Sum(), want)
		}
	}
	if at != len(data) {
		t.Fatalf("reassembled %d of %d bytes", at, len(data))
	}
}

func TestChunkerOneByteReader(t *testing.T) {
	data := randData(120 * 1024)
	want := collect(t, ramcdc.New(bytes.NewReader(data), 512, 4*1024))
	got := collect(t, ramcdc.New(iotest.OneByteReader(bytes.NewReader(data)), 512, 4*1024))
	if !sameChunks(got, want) {
		t.Fatalf("one-byte reader: got %d chunks, want %d", len(got), len(want))
	}
}

func TestChunkerReset(t *testing.T) {
	a := randData(80 * 1024)
	b := structData(90 * 1024)
	c := ramcdc.New(bytes.NewReader(a), 512, 4*1024)
	got1 := collect(t, c)
	c.Reset(bytes.NewReader(b))
	got2 := collect(t, c)
	if !sameChunks(got1, collect(t, ramcdc.New(bytes.NewReader(a), 512, 4*1024))) {
		t.Fatal("first pass changed under Reset")
	}
	if !sameChunks(got2, collect(t, ramcdc.New(bytes.NewReader(b), 512, 4*1024))) {
		t.Fatal("post-Reset pass differs from a fresh chunker")
	}
}

func TestChunkerReaderError(t *testing.T) {
	boom := errors.New("boom")
	c := ramcdc.New(iotest.ErrReader(boom), 64, 256)
	if c.Next() {
		t.Fatal("expected Next false on reader error")
	}
	if !errors.Is(c.Err(), boom) {
		t.Fatalf("Err = %v, want boom", c.Err())
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
	mustPanic("windowSize < 1", func() { ramcdc.New(bytes.NewReader(nil), 0, 64) })
	mustPanic("maxSize < windowSize", func() { ramcdc.New(bytes.NewReader(nil), 64, 32) })
}

func BenchmarkChunker(b *testing.B) {
	data := randData(1 << 20)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	c := ramcdc.New(nil, 8<<10-256, 64<<10)
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
