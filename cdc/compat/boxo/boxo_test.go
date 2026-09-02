package boxo_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"testing"
	"testing/iotest"

	boxo "github.com/chmduquesne/rollinghash/v4/cdc/compat/boxo"
)

func xorshiftData(n int) []byte {
	b := make([]byte, n)
	var x uint64 = 0x9E3779B97F4A7C15
	for i := range b {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		b[i] = byte(x)
	}
	return b
}

func drain(t *testing.T, s boxo.Splitter) [][]byte {
	t.Helper()
	var out [][]byte
	for {
		b, err := s.NextBytes()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("NextBytes: %v", err)
		}
		out = append(out, append([]byte(nil), b...))
	}
}

func lengths(chunks [][]byte) []int {
	l := make([]int, len(chunks))
	for i, c := range chunks {
		l[i] = len(c)
	}
	return l
}

func contentDigest(chunks [][]byte) string {
	h := sha256.New()
	for _, c := range chunks {
		h.Write(c)
	}
	return fmt.Sprintf("%x", h.Sum(nil)[:8])
}

func reassemble(t *testing.T, chunks [][]byte, want []byte) {
	t.Helper()
	var got []byte
	for _, c := range chunks {
		got = append(got, c...)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("reassembled %d bytes, want %d", len(got), len(want))
	}
}

// TestGoldenRabin and TestGoldenBuzhash pin exact chunk lengths and a content
// digest for a fixed input. The values were produced by the real
// github.com/ipfs/boxo/chunker and verified byte-identical (see
// cdc/compat/boxo/bench). No external dependency here.
func TestGoldenRabin(t *testing.T) {
	data := xorshiftData(200000)
	chunks := drain(t, boxo.NewRabinMinMax(bytes.NewReader(data), 2048, 8192, 32768))

	wantLens := []int{7914, 32768, 2474, 11033, 6579, 10845, 11978, 14825, 3503, 3182,
		3478, 4505, 31546, 13292, 7217, 5358, 4997, 2998, 5732, 12844, 2932}
	if got := lengths(chunks); fmt.Sprint(got) != fmt.Sprint(wantLens) {
		t.Fatalf("chunk lengths:\n got %v\nwant %v", got, wantLens)
	}
	if got := contentDigest(chunks); got != "6705af8841c84dbe" {
		t.Fatalf("content digest = %s, want 6705af8841c84dbe", got)
	}
	reassemble(t, chunks, data)
}

func TestGoldenBuzhash(t *testing.T) {
	data := xorshiftData(700000)
	chunks := drain(t, boxo.NewBuzhash(bytes.NewReader(data)))

	wantLens := []int{162520, 176506, 204908, 156066}
	if got := lengths(chunks); fmt.Sprint(got) != fmt.Sprint(wantLens) {
		t.Fatalf("chunk lengths:\n got %v\nwant %v", got, wantLens)
	}
	if got := contentDigest(chunks); got != "c2f514e87137e8ab" {
		t.Fatalf("content digest = %s, want c2f514e87137e8ab", got)
	}
	reassemble(t, chunks, data)
	for i, c := range chunks[:len(chunks)-1] {
		if len(c) < 128<<10 || len(c) > 512<<10 {
			t.Errorf("chunk %d length %d outside [128 KiB, 512 KiB]", i, len(c))
		}
	}
}

func TestSizeSplitter(t *testing.T) {
	data := xorshiftData(1000)
	chunks := drain(t, boxo.NewSizeSplitter(bytes.NewReader(data), 256))
	if got := lengths(chunks); fmt.Sprint(got) != fmt.Sprint([]int{256, 256, 256, 232}) {
		t.Fatalf("lengths %v", got)
	}
	reassemble(t, chunks, data)

	// exact multiple
	chunks = drain(t, boxo.NewSizeSplitter(bytes.NewReader(xorshiftData(512)), 256))
	if got := lengths(chunks); fmt.Sprint(got) != fmt.Sprint([]int{256, 256}) {
		t.Fatalf("exact multiple: lengths %v", got)
	}

	// empty
	if chunks = drain(t, boxo.NewSizeSplitter(bytes.NewReader(nil), 256)); chunks != nil {
		t.Fatalf("empty: got %d chunks", len(chunks))
	}
}

func TestRabinBounds(t *testing.T) {
	data := xorshiftData(300000)
	// NewRabin derives min = avg/3, max = avg + avg/2.
	chunks := drain(t, boxo.NewRabin(bytes.NewReader(data), 6000))
	reassemble(t, chunks, data)
	for i, c := range chunks[:len(chunks)-1] {
		if len(c) < 2000 || len(c) > 9000 {
			t.Errorf("chunk %d length %d outside [avg/3, avg*3/2]", i, len(c))
		}
	}
}

func TestFromString(t *testing.T) {
	data := xorshiftData(50000)

	for _, tc := range []struct {
		spec      string
		minChunks int
	}{
		{"", 1},
		{"default", 1},
		{"size-1024", 48},
		{"rabin", 1},
		{"rabin-4096", 5},
		{"rabin-min:512-avg:4096-max:16384", 5},
		{"buzhash", 1},
	} {
		s, err := boxo.FromString(bytes.NewReader(data), tc.spec)
		if err != nil {
			t.Fatalf("FromString(%q): %v", tc.spec, err)
		}
		chunks := drain(t, s)
		if len(chunks) < tc.minChunks {
			t.Errorf("FromString(%q): %d chunks, want >= %d", tc.spec, len(chunks), tc.minChunks)
		}
		reassemble(t, chunks, data)
	}

	for _, bad := range []string{"nope", "size-0", "size-abc", "rabin-1-2", "rabin-min:20-avg:10-max:5"} {
		if _, err := boxo.FromString(bytes.NewReader(data), bad); err == nil {
			t.Errorf("FromString(%q): expected error", bad)
		}
	}
}

func TestRegister(t *testing.T) {
	boxo.Register("boxotestchunker", func(r io.Reader, _ string) (boxo.Splitter, error) {
		return boxo.NewSizeSplitter(r, 128), nil
	})
	s, err := boxo.FromString(bytes.NewReader(xorshiftData(300)), "boxotestchunker-ignored")
	if err != nil {
		t.Fatal(err)
	}
	if got := lengths(drain(t, s)); fmt.Sprint(got) != fmt.Sprint([]int{128, 128, 44}) {
		t.Fatalf("registered chunker lengths %v", got)
	}

	for _, name := range []string{"", "has-dash", "size"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Register(%q): expected panic", name)
				}
			}()
			boxo.Register(name, func(io.Reader, string) (boxo.Splitter, error) { return nil, nil })
		}()
	}
}

func TestReaderAndError(t *testing.T) {
	r := bytes.NewReader(xorshiftData(10))
	if boxo.NewBuzhash(r).Reader() != r {
		t.Error("Buzhash.Reader mismatch")
	}
	if boxo.NewRabin(r, 4096).Reader() != r {
		t.Error("Rabin.Reader mismatch")
	}

	boom := errors.New("boom")
	for name, s := range map[string]boxo.Splitter{
		"size":    boxo.NewSizeSplitter(iotest.ErrReader(boom), 256),
		"rabin":   boxo.NewRabin(iotest.ErrReader(boom), 4096),
		"buzhash": boxo.NewBuzhash(iotest.ErrReader(boom)),
	} {
		if _, err := s.NextBytes(); !errors.Is(err, boom) {
			t.Errorf("%s: err = %v, want boom", name, err)
		}
	}
}

func TestChan(t *testing.T) {
	data := xorshiftData(40000)
	out, errs := boxo.Chan(boxo.NewRabin(bytes.NewReader(data), 4096))
	var got []byte
	for c := range out {
		got = append(got, c...)
	}
	if err := <-errs; !errors.Is(err, io.EOF) {
		t.Fatalf("Chan error = %v, want io.EOF", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("Chan reassembled %d bytes, want %d", len(got), len(data))
	}
}
