package ultracdc_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	"github.com/chmduquesne/rollinghash/v4/cdc/internal/plakar"
	"github.com/chmduquesne/rollinghash/v4/cdc/ultracdc"
)

func randData(n int) []byte {
	data := make([]byte, n)
	var x uint64 = 0x0f1e2d3c4b5a6978
	for i := range data {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		data[i] = byte(x)
	}
	return data
}

// lowEntropyData embeds a long run of one repeated 8-byte window to exercise
// the low-entropy-string cut path.
func lowEntropyData(n int) []byte {
	data := randData(n)
	if n > 40*1024 {
		for i := 8 * 1024; i < 34*1024; i++ {
			data[i] = 0x5c
		}
	}
	return data
}

func collect(t *testing.T, c *ultracdc.Chunker) [][]byte {
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

func TestChunkerMatchesPlakar(t *testing.T) {
	configs := []plakar.Opts{
		{MinSize: 2 * 1024, NormalSize: 10 * 1024, MaxSize: 64 * 1024},
		{MinSize: 512, NormalSize: 4 * 1024, MaxSize: 32 * 1024},
		{MinSize: 128, NormalSize: 1024, MaxSize: 8 * 1024},
	}
	datasets := map[string][]byte{
		"rand300k":   randData(300 * 1024),
		"lowentropy": lowEntropyData(300 * 1024),
		"rand1k":     randData(1024),
		"tail":       randData(70*1024 + 55),
		"shorterMin": randData(40),
	}
	for _, cfg := range configs {
		for _, spec := range []bool{false, true} {
			algo := func(d []byte, n int) int {
				return plakar.UltraCDCAlgorithm(spec, cfg, d, n)
			}
			for dname, data := range datasets {
				want := plakar.Chunks(algo, data, cfg)

				var opts []ultracdc.Option
				if spec {
					opts = append(opts, ultracdc.WithSpecFaithful())
				}
				c := ultracdc.New(bytes.NewReader(data), cfg.MinSize, cfg.NormalSize, cfg.MaxSize, opts...)
				got := collect(t, c)

				if len(got) != len(want) {
					t.Fatalf("%v spec=%v/%s: got %d chunks, want %d", cfg, spec, dname, len(got), len(want))
				}
				for i := range want {
					if !bytes.Equal(got[i], want[i]) {
						t.Fatalf("%v spec=%v/%s chunk %d: got %d bytes, want %d bytes",
							cfg, spec, dname, i, len(got[i]), len(want[i]))
					}
				}
				if joined := bytes.Join(got, nil); !bytes.Equal(joined, data) {
					t.Fatalf("%v spec=%v/%s: reassembled %d bytes, want %d", cfg, spec, dname, len(joined), len(data))
				}
			}
		}
	}
}

func TestHammingTableMatchesPlakar(t *testing.T) {
	// Indirectly: a low-entropy dataset that leans on the running-distance
	// update would diverge if our table were wrong; TestChunkerMatchesPlakar
	// already covers that. This just pins the reference table shape.
	got := plakar.HammingTo0xAA()
	for i, v := range got {
		if v < 0 || v > 8 {
			t.Fatalf("hamming[%d] = %d out of range", i, v)
		}
	}
	if got[0xAA] != 0 || got[0x55] != 8 {
		t.Fatalf("hamming[0xAA]=%d hamming[0x55]=%d", got[0xAA], got[0x55])
	}
}

func TestChunkerDeterminism(t *testing.T) {
	data := randData(200 * 1024)
	mk := func(r io.Reader) [][]byte {
		return collect(t, ultracdc.New(r, 2*1024, 10*1024, 64*1024))
	}
	want := mk(bytes.NewReader(data))
	got := mk(iotest.OneByteReader(bytes.NewReader(data)))
	if len(got) != len(want) {
		t.Fatalf("onebyte: got %d chunks, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("onebyte chunk %d differs", i)
		}
	}
}

func TestChunkerEdgeCases(t *testing.T) {
	// Empty input: no chunks, zero-value accessors.
	c := ultracdc.New(bytes.NewReader(nil), 64, 1024, 8192)
	if c.Next() {
		t.Error("empty: expected no chunks")
	}
	if c.Bytes() != nil || c.Sum() != 0 || c.ContentDefined() {
		t.Error("empty: expected zero-value accessors")
	}

	// A stream shorter than the 8-byte UltraCDC window still yields its bytes as
	// one final, non-content-defined chunk (matches rollinghash.Chunker as of
	// v4.3.3). Sum stays 0: no full window ends at the cut.
	subWindow := randData(7)
	c = ultracdc.New(bytes.NewReader(subWindow), 64, 1024, 8192)
	if !c.Next() {
		t.Fatal("sub-window: expected one chunk")
	}
	if !bytes.Equal(c.Bytes(), subWindow) || c.ContentDefined() || c.Sum() != 0 {
		t.Errorf("sub-window: chunk=%d bytes contentDefined=%v sum=%d, want %d bytes / false / 0",
			len(c.Bytes()), c.ContentDefined(), c.Sum(), len(subWindow))
	}
	if c.Next() {
		t.Error("sub-window: expected exactly one chunk")
	}

	// A short (sub-min) stream longer than the window: still one final chunk of
	// the whole input.
	short := randData(50)
	c = ultracdc.New(bytes.NewReader(short), 64, 1024, 8192)
	if got := collect(t, c); len(got) != 1 || !bytes.Equal(got[0], short) {
		t.Errorf("short: expected one chunk of the whole input, got %d chunks", len(got))
	}

	// Exactly the window size: still one chunk of the whole input.
	exact := randData(8)
	c = ultracdc.New(bytes.NewReader(exact), 64, 1024, 8192)
	if got := collect(t, c); len(got) != 1 || !bytes.Equal(got[0], exact) {
		t.Errorf("exactly-window: expected one chunk of the whole input, got %d chunks", len(got))
	}
}

func TestChunkerError(t *testing.T) {
	boom := errors.New("boom")
	c := ultracdc.New(iotest.ErrReader(boom), 128, 1024, 8192)
	if c.Next() {
		t.Error("expected Next to fail on reader error")
	}
	if !errors.Is(c.Err(), boom) {
		t.Errorf("expected Err to be boom, got %v", c.Err())
	}
}

func TestChunkerReset(t *testing.T) {
	data := randData(200 * 1024)
	c := ultracdc.New(bytes.NewReader(data), 2*1024, 10*1024, 64*1024)
	want := collect(t, c)
	c.Reset(bytes.NewReader(data))
	got := collect(t, c)
	if len(got) != len(want) {
		t.Fatalf("after Reset: got %d chunks, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("chunk %d differs after Reset", i)
		}
	}
}

func BenchmarkChunker(b *testing.B) {
	data := randData(1 << 20)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	c := ultracdc.New(nil, 2<<10, 10<<10, 64<<10)
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
