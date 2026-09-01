package fastcdc_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	"github.com/chmduquesne/rollinghash/v4/cdc/fastcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/plakar"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
)

func randData(n int) []byte {
	data := make([]byte, n)
	var x uint64 = 0x123456789abcdef0
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

func collect(t *testing.T, c *fastcdc.Chunker) [][]byte {
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
		{MinSize: 2 * 1024, NormalSize: 8 * 1024, MaxSize: 64 * 1024},
		{MinSize: 512, NormalSize: 4 * 1024, MaxSize: 32 * 1024},
		{MinSize: 256, NormalSize: 2 * 1024, MaxSize: 16 * 1024},
		{MinSize: 64, NormalSize: 1024, MaxSize: 8 * 1024},
	}
	datasets := map[string][]byte{
		"rand300k":   randData(300 * 1024),
		"struct200k": structData(200 * 1024),
		"rand1k":     randData(1024),
		"tail":       randData(70*1024 + 77),
		"shorterMin": randData(40),
	}
	variants := []struct {
		name   string
		legacy bool
	}{
		{"fastcdc", true},
		{"fastcdc-v1.0.0", false},
	}

	g := plakar.GearTable
	for _, cfg := range configs {
		for _, v := range variants {
			maskS, maskL := plakar.FastCDCSetupMasks(cfg, v.legacy)
			algo := func(d []byte, n int) int {
				return plakar.FastCDCAlgorithm(&g, maskS, maskL, cfg, d, n)
			}
			for dname, data := range datasets {
				want := plakar.Chunks(algo, data, cfg)

				c := fastcdc.New(bytes.NewReader(data),
					gearhash64.NewFromUint64Array(plakar.GearTable),
					cfg.MinSize, cfg.NormalSize, cfg.MaxSize,
					fastcdc.WithMasks(maskS, maskL))
				got := collect(t, c)

				if len(got) != len(want) {
					t.Fatalf("%v/%s/%s: got %d chunks, want %d", cfg, v.name, dname, len(got), len(want))
				}
				for i := range want {
					if !bytes.Equal(got[i], want[i]) {
						t.Fatalf("%v/%s/%s chunk %d: got %d bytes, want %d bytes",
							cfg, v.name, dname, i, len(got[i]), len(want[i]))
					}
				}
				if joined := bytes.Join(got, nil); !bytes.Equal(joined, data) {
					t.Fatalf("%v/%s/%s: reassembled %d bytes, want %d", cfg, v.name, dname, len(joined), len(data))
				}
			}
		}
	}
}

// TestDerivedMasksMatchPlakar checks that New's own mask derivation (no
// WithMasks) equals plakar's calculateMasks for non-default sizes.
func TestDerivedMasksMatchPlakar(t *testing.T) {
	cfg := plakar.Opts{MinSize: 512, NormalSize: 4 * 1024, MaxSize: 32 * 1024}
	data := randData(200 * 1024)

	g := plakar.GearTable
	maskS, maskL := plakar.FastCDCMasks(cfg.NormalSize, plakar.FastCDCNormalLevel)
	algo := func(d []byte, n int) int {
		return plakar.FastCDCAlgorithm(&g, maskS, maskL, cfg, d, n)
	}
	want := plakar.Chunks(algo, data, cfg)

	c := fastcdc.New(bytes.NewReader(data), gearhash64.NewFromUint64Array(plakar.GearTable),
		cfg.MinSize, cfg.NormalSize, cfg.MaxSize)
	got := collect(t, c)

	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("chunk %d differs", i)
		}
	}
}

func TestChunkerDeterminism(t *testing.T) {
	data := randData(200 * 1024)
	mk := func(r io.Reader) [][]byte {
		return collect(t, fastcdc.New(r, gearhash64.New(), 2*1024, 8*1024, 64*1024))
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
	c := fastcdc.New(bytes.NewReader(nil), gearhash64.New(), 64, 256, 1024)
	if c.Next() {
		t.Error("empty: expected no chunks")
	}

	data := structData(30)
	c = fastcdc.New(bytes.NewReader(data), gearhash64.New(), 64, 256, 1024)
	got := collect(t, c)
	if len(got) != 1 || !bytes.Equal(got[0], data) {
		t.Errorf("short: expected one chunk of all data, got %d chunks", len(got))
	}
}

func TestChunkerError(t *testing.T) {
	boom := errors.New("boom")
	c := fastcdc.New(iotest.ErrReader(boom), gearhash64.New(), 64, 256, 1024)
	if c.Next() {
		t.Error("expected Next to fail on reader error")
	}
	if !errors.Is(c.Err(), boom) {
		t.Errorf("expected Err to be boom, got %v", c.Err())
	}
}

func TestChunkerReset(t *testing.T) {
	data := randData(200 * 1024)
	c := fastcdc.New(bytes.NewReader(data), gearhash64.New(), 2*1024, 8*1024, 64*1024)
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

func TestNewPanicsWithoutTable(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic for a hash without Table()")
		}
	}()
	fastcdc.New(bytes.NewReader(nil), notAGearHash{}, 64, 256, 1024)
}

type notAGearHash struct{}

func (notAGearHash) Write(p []byte) (int, error)        { return len(p), nil }
func (notAGearHash) Sum(b []byte) []byte                { return b }
func (notAGearHash) Reset()                             {}
func (notAGearHash) Size() int                          { return 8 }
func (notAGearHash) BlockSize() int                     { return 1 }
func (notAGearHash) Sum64() uint64                      { return 0 }
func (notAGearHash) Roll(b byte)                        {}
func (notAGearHash) WriteWindow(io.Writer) (int, error) { return 0, nil }

func BenchmarkChunker(b *testing.B) {
	data := randData(1 << 20)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	c := fastcdc.New(nil, gearhash64.New(), 2<<10, 8<<10, 64<<10)
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
