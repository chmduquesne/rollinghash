package repmaxsfxcdc_test

import (
	"bytes"
	"errors"
	"testing"
	"testing/iotest"

	"github.com/chmduquesne/rollinghash/v4/cdc/repmaxsfxcdc"
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

func collect(t *testing.T, c *repmaxsfxcdc.Chunker) [][]byte {
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

// refSfx is an independent whole-buffer transcription of buildbarn/go-cdc's
// simpleRepMaxSfxChunkReader.ReadNextChunk (HEAD, unreleased), used as the
// oracle. buildbarn ships NewSimpleRepMaxSfxContentDefinedChunker precisely as
// the reference for NewRepMaxSfxContentDefinedChunker (with the identity
// substitution box). Keep this in sync with upstream if the algorithm changes.
func refSfx(data []byte, min, horizon int) [][]byte {
	peek := 2*min + horizon
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
		var n int
		for {
			best := min
			for i := min + 1; i <= len(d)-min; i++ {
				if bytes.Compare(d[best:best+min], d[i:i+min]) < 0 {
					best = i
				}
			}
			if best < 2*min {
				n = best
				break
			}
			d = d[:best]
		}
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

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestChunkerStatic runs every case from buildbarn/go-cdc's
// TestRepMaxSfxContentDefinedChunkerNewChunkReader/Static.
func TestChunkerStatic(t *testing.T) {
	for _, tc := range []struct {
		min, horizon int
		data         string
		want         []int
	}{
		{2, 40, "01210", []int{2, 3}},
		{2, 40, "01234", []int{3, 2}},
		{2, 0, "01234", []int{2, 3}},
		{3, 10, "0000001", []int{4, 3}},
		{5, 23, "0000000000100000", []int{5, 5, 6}},
		{5, 23, "00000001000", []int{6, 5}},
		{2, 82, "0000120", []int{3, 2, 2}},
		{15, 44, "00000000000000000000000000000001", []int{17, 15}},
		{7, 17, "000000011111111", []int{7, 8}},
		{7, 17, "000000000000011110011", []int{13, 8}},
		{7, 17, "000000022222220000001", []int{7, 7, 7}},
		{7, 17, "000000011111111111111", []int{7, 7, 7}},
		{7, 17, "00000000010002", []int{7, 7}},
		{2, 17, "0000010", []int{2, 3, 2}},
		{2, 224, "0000001", []int{2, 3, 2}},
		{4, 106, "0000010101010110000000000", []int{5, 4, 4, 4, 4, 4}},
		{2, 3, "00001120", []int{3, 3, 2}},
		{7, 17, "00000000100100120000000", []int{8, 7, 8}},
		{10, 84, "000000000011011011112000000", []int{17, 10}},
		{28, 29, "0000000000000000000000000000100100100010010010001001001001001010000000000000000", []int{51, 28}},
	} {
		got := lens(collect(t, repmaxsfxcdc.New(bytes.NewReader([]byte(tc.data)), tc.min, tc.horizon)))
		if !equalInts(got, tc.want) {
			t.Errorf("min=%d horizon=%d %q: cuts %v, want %v", tc.min, tc.horizon, tc.data, got, tc.want)
		}
		// The oracle must agree too.
		if ref := lens(refSfx([]byte(tc.data), tc.min, tc.horizon)); !equalInts(ref, tc.want) {
			t.Errorf("min=%d horizon=%d %q: refSfx cuts %v, want %v", tc.min, tc.horizon, tc.data, ref, tc.want)
		}
	}
}

func TestChunkerMatchesReference(t *testing.T) {
	configs := []struct{ min, horizon int }{
		{2, 0},
		{16, 64},
		{256, 512},
		{512, 2 * 1024},
		{1024, 4 * 1024},
	}
	datasets := map[string][]byte{
		"rand300k":   randData(300 * 1024),
		"struct200k": structData(200 * 1024),
		"lowcard":    lowCardData(200 * 1024),
		"rand1k":     randData(1024),
		"tail":       randData(40*1024 + 777),
		"tiny":       randData(3),
		"empty":      nil,
	}

	for _, cfg := range configs {
		for name, data := range datasets {
			want := refSfx(data, cfg.min, cfg.horizon)
			got := collect(t, repmaxsfxcdc.New(bytes.NewReader(data), cfg.min, cfg.horizon))

			if !sameChunks(got, want) {
				t.Fatalf("min=%d horizon=%d %s: %d chunks, want %d (got=%v want=%v)",
					cfg.min, cfg.horizon, name, len(got), len(want), lens(got), lens(want))
			}
			if joined := bytes.Join(got, nil); !bytes.Equal(joined, data) {
				t.Fatalf("min=%d horizon=%d %s: reassembled %d bytes, want %d", cfg.min, cfg.horizon, name, len(joined), len(data))
			}
			for i, ch := range got {
				last := i == len(got)-1
				if !last && (len(ch) < cfg.min || len(ch) >= 2*cfg.min) {
					t.Fatalf("min=%d horizon=%d %s: chunk %d has %d bytes, want [%d, %d)",
						cfg.min, cfg.horizon, name, i, len(ch), cfg.min, 2*cfg.min)
				}
			}
		}
	}
}

// lowCardData uses a 4-symbol alphabet with runs, exercising the lexicographic
// comparison's non-linear worst case and the restriction loop.
func lowCardData(n int) []byte {
	data := make([]byte, n)
	var x uint64 = 0x243f6a8885a308d3
	run := 0
	var cur byte
	for i := range data {
		if run == 0 {
			x ^= x << 13
			x ^= x >> 7
			x ^= x << 17
			cur = "abcd"[x&3]
			run = 1 + int((x>>2)&31)
		}
		data[i] = cur
		run--
	}
	return data
}

func TestChunkerOneByteReader(t *testing.T) {
	data := randData(120 * 1024)
	want := collect(t, repmaxsfxcdc.New(bytes.NewReader(data), 512, 2*1024))
	got := collect(t, repmaxsfxcdc.New(iotest.OneByteReader(bytes.NewReader(data)), 512, 2*1024))
	if !sameChunks(got, want) {
		t.Fatalf("one-byte reader: got %d chunks, want %d", len(got), len(want))
	}
}

func TestChunkerReset(t *testing.T) {
	a := randData(80 * 1024)
	b := lowCardData(90 * 1024)
	c := repmaxsfxcdc.New(bytes.NewReader(a), 512, 2*1024)
	got1 := collect(t, c)
	c.Reset(bytes.NewReader(b))
	got2 := collect(t, c)

	if !sameChunks(got1, collect(t, repmaxsfxcdc.New(bytes.NewReader(a), 512, 2*1024))) {
		t.Fatal("first pass changed under Reset")
	}
	if !sameChunks(got2, collect(t, repmaxsfxcdc.New(bytes.NewReader(b), 512, 2*1024))) {
		t.Fatal("post-Reset pass differs from a fresh chunker")
	}
}

func TestChunkerReaderError(t *testing.T) {
	boom := errors.New("boom")
	c := repmaxsfxcdc.New(iotest.ErrReader(boom), 16, 64)
	if c.Next() {
		t.Fatal("expected Next false on reader error")
	}
	if !errors.Is(c.Err(), boom) {
		t.Fatalf("Err = %v, want boom", c.Err())
	}
}

func TestChunkerSum(t *testing.T) {
	data := randData(200 * 1024)
	c := repmaxsfxcdc.New(bytes.NewReader(data), 512, 2*1024)
	at := 0
	for c.Next() {
		ch := c.Bytes()
		at += len(ch)
		var want uint64
		if c.ContentDefined() {
			want = uint64(data[at]) // first byte of the next chunk
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

func BenchmarkChunker(b *testing.B) {
	data := randData(1 << 20)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	c := repmaxsfxcdc.New(nil, 4<<10, 4<<10)
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
	mustPanic("minSize too small", func() { repmaxsfxcdc.New(bytes.NewReader(nil), 1, 0) })
	mustPanic("negative horizon", func() { repmaxsfxcdc.New(bytes.NewReader(nil), 16, -1) })
}
