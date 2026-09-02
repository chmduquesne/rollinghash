package gocdc_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/chmduquesne/rollinghash/v4/cdc/compat/gocdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/plakar"
)

func randData(n int) []byte {
	data := make([]byte, n)
	var x uint64 = 0xcafef00dd15ea5e5
	for i := range data {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		data[i] = byte(x)
	}
	return data
}

// oracle returns the plakar chunk list for algorithm over data at o.
func oracle(t *testing.T, algorithm string, data []byte, o plakar.Opts) [][]byte {
	t.Helper()
	g := plakar.GearTable
	var algo plakar.Algorithm
	switch algorithm {
	case "fastcdc", "fastcdc-v1.0.0":
		maskS, maskL := plakar.FastCDCSetupMasks(o, algorithm == "fastcdc")
		algo = func(d []byte, n int) int { return plakar.FastCDCAlgorithm(&g, maskS, maskL, o, d, n) }
	case "jc", "jc-v1.0.0", "jc-v1.1.0":
		maskC, maskJ, jumpLen := plakar.JCSetup(o, algorithm == "jc" || algorithm == "jc-v1.1.0")
		spec := algorithm == "jc-v1.1.0"
		algo = func(d []byte, n int) int {
			return plakar.JCAlgorithm(&g, maskC, maskJ, jumpLen, spec, o, d, n)
		}
	case "ultracdc", "ultracdc-v1.0.0":
		spec := algorithm == "ultracdc-v1.0.0"
		algo = func(d []byte, n int) int { return plakar.UltraCDCAlgorithm(spec, o, d, n) }
	default:
		t.Fatalf("no oracle for %q", algorithm)
	}
	return plakar.Chunks(algo, data, o)
}

func drain(t *testing.T, c *gocdc.Chunker) [][]byte {
	t.Helper()
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
			t.Fatalf("Next: %v", err)
		}
	}
}

func plakarDefaults(algorithm string) plakar.Opts {
	if algorithm[:2] == "ul" {
		return plakar.Opts{MinSize: 2 * 1024, NormalSize: 10 * 1024, MaxSize: 64 * 1024}
	}
	return plakar.Opts{MinSize: 2 * 1024, NormalSize: 8 * 1024, MaxSize: 64 * 1024}
}

func TestNewChunkerMatchesPlakar(t *testing.T) {
	names := []string{
		"fastcdc", "fastcdc-v1.0.0",
		"jc", "jc-v1.0.0", "jc-v1.1.0",
		"ultracdc", "ultracdc-v1.0.0",
	}
	cases := []struct {
		name string
		opts *gocdc.ChunkerOpts
	}{
		{"defaults", nil},
		{"custom", &gocdc.ChunkerOpts{MinSize: 512, NormalSize: 4 * 1024, MaxSize: 32 * 1024}},
	}
	datasets := map[string][]byte{
		"rand256k": randData(256 * 1024),
		"rand900":  randData(900),
		"tail":     randData(66*1024 + 33),
	}

	for _, algo := range names {
		for _, cs := range cases {
			po := plakarDefaults(algo)
			if cs.opts != nil {
				po = plakar.Opts{MinSize: cs.opts.MinSize, NormalSize: cs.opts.NormalSize, MaxSize: cs.opts.MaxSize}
			}
			for dname, data := range datasets {
				want := oracle(t, algo, data, po)
				c, err := gocdc.NewChunker(algo, bytes.NewReader(data), cs.opts)
				if err != nil {
					t.Fatalf("%s/%s: NewChunker: %v", algo, cs.name, err)
				}
				got := drain(t, c)
				if len(got) != len(want) {
					t.Fatalf("%s/%s/%s: got %d chunks, want %d", algo, cs.name, dname, len(got), len(want))
				}
				for i := range want {
					if !bytes.Equal(got[i], want[i]) {
						t.Fatalf("%s/%s/%s chunk %d: got %d bytes, want %d bytes",
							algo, cs.name, dname, i, len(got[i]), len(want[i]))
					}
				}
				if joined := bytes.Join(got, nil); !bytes.Equal(joined, data) {
					t.Fatalf("%s/%s/%s: reassembled %d bytes, want %d", algo, cs.name, dname, len(joined), len(data))
				}
			}
		}
	}
}

func TestNewChunkerBuffer(t *testing.T) {
	data := randData(200 * 1024)
	buf := make([]byte, 0, 4*64*1024)

	c1, _ := gocdc.NewChunker("fastcdc", bytes.NewReader(data), nil)
	want := drain(t, c1)

	c2, err := gocdc.NewChunkerBuffer("fastcdc", bytes.NewReader(data), nil, buf)
	if err != nil {
		t.Fatal(err)
	}
	got := drain(t, c2)

	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("chunk %d differs with supplied buffer", i)
		}
	}
}

func TestSplitOffsets(t *testing.T) {
	data := randData(200 * 1024)
	c, err := gocdc.NewChunker("fastcdc", bytes.NewReader(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	var at uint
	err = c.Split(func(offset, length uint, chunk []byte) error {
		if offset != at {
			t.Fatalf("offset %d, want %d", offset, at)
		}
		if length != uint(len(chunk)) {
			t.Fatalf("length %d != len(chunk) %d", length, len(chunk))
		}
		if !bytes.Equal(chunk, data[offset:offset+length]) {
			t.Fatalf("chunk at %d does not match source", offset)
		}
		at += length
		return nil
	})
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if at != uint(len(data)) {
		t.Fatalf("Split covered %d bytes, want %d", at, len(data))
	}
}

func TestCopy(t *testing.T) {
	data := randData(150 * 1024)
	c, err := gocdc.NewChunker("jc", bytes.NewReader(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	n, err := c.Copy(&buf)
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if n != int64(len(data)) || !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("Copy wrote %d bytes, want %d (equal=%v)", n, len(data), bytes.Equal(buf.Bytes(), data))
	}
}

func TestSplitPropagatesError(t *testing.T) {
	sentinel := errors.New("stop")
	c, _ := gocdc.NewChunker("ultracdc", bytes.NewReader(randData(100*1024)), nil)
	err := c.Split(func(uint, uint, []byte) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("Split error = %v, want %v", err, sentinel)
	}
}

func TestUnknownKeyedAndInvalid(t *testing.T) {
	if _, err := gocdc.NewChunker("bogus", bytes.NewReader(nil), nil); err == nil {
		t.Error("expected error for unknown algorithm")
	}
	if _, err := gocdc.NewChunker("kfastcdc", bytes.NewReader(nil), nil); err == nil {
		t.Error("expected error for keyed variant")
	}
	if _, err := gocdc.NewChunker("fastcdc", bytes.NewReader(nil), &gocdc.ChunkerOpts{Key: []byte("k")}); err == nil {
		t.Error("expected error when Key is set")
	}
	// FastCDC requires a power-of-two NormalSize.
	if _, err := gocdc.NewChunker("fastcdc", bytes.NewReader(nil), &gocdc.ChunkerOpts{
		MinSize: 512, NormalSize: 5000, MaxSize: 32 * 1024,
	}); !errors.Is(err, gocdc.ErrNormalSize) {
		t.Errorf("expected ErrNormalSize, got %v", err)
	}
	// MinSize >= NormalSize.
	if _, err := gocdc.NewChunker("jc", bytes.NewReader(nil), &gocdc.ChunkerOpts{
		MinSize: 8192, NormalSize: 4096, MaxSize: 64 * 1024,
	}); !errors.Is(err, gocdc.ErrMinSize) {
		t.Errorf("expected ErrMinSize, got %v", err)
	}
}
