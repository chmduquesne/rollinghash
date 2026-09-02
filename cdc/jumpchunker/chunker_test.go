package jumpchunker_test

import (
	"bytes"
	"errors"
	"testing"
	"testing/iotest"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/plakar"
	"github.com/chmduquesne/rollinghash/v4/cdc/jumpchunker"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
)

func testData(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i*2654435761 + i/7)
	}
	return data
}

// randData fills a slice with xorshift64 pseudo-random bytes; the deterministic
// testData formula produces almost no natural JC boundaries.
func randData(n int) []byte {
	data := make([]byte, n)
	var x uint64 = 0xdeadbeefcafe1234
	for i := range data {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		data[i] = byte(x)
	}
	return data
}

func collect(t *testing.T, c *jumpchunker.Chunker) [][]byte {
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

// plakarJC returns the oracle chunk list for legacy plakar "jc" over data, plus
// a jumpchunker configured to match it.
func plakarJC(r *bytes.Reader, o plakar.Opts, legacy, specFaithful bool) (algo plakar.Algorithm, mkChunker func() *jumpchunker.Chunker) {
	maskC, maskJ, jumpLen := plakar.JCSetup(o, legacy)
	g := plakar.GearTable
	algo = func(d []byte, n int) int {
		return plakar.JCAlgorithm(&g, maskC, maskJ, jumpLen, specFaithful, o, d, n)
	}
	mkChunker = func() *jumpchunker.Chunker {
		opts := []jumpchunker.Option{jumpchunker.WithJumpMask(maskC, jumpLen)}
		if specFaithful {
			opts = append(opts, jumpchunker.WithSpecFaithful())
		}
		return jumpchunker.New(r, gearhash64.NewFromUint64Array(plakar.GearTable),
			o.NormalSize, o.MinSize, o.MaxSize, opts...)
	}
	return
}

// TestChunkerMatchesPlakar asserts byte-for-byte equality with the ported
// plakar JC algorithm across several size configs, variants, and data shapes.
func TestChunkerMatchesPlakar(t *testing.T) {
	configs := []plakar.Opts{
		{MinSize: 2 * 1024, NormalSize: 8 * 1024, MaxSize: 64 * 1024}, // legacy default
		{MinSize: 512, NormalSize: 4 * 1024, MaxSize: 32 * 1024},
		{MinSize: 256, NormalSize: 2 * 1024, MaxSize: 16 * 1024},
		{MinSize: 64, NormalSize: 1024, MaxSize: 8 * 1024},
	}
	datasets := map[string][]byte{
		"rand300k":   randData(300 * 1024),
		"struct200k": testData(200 * 1024),
		"rand1k":     randData(1024),
		"tail":       randData(70*1024 + 111),
		"shorterMin": randData(40),
	}
	variants := []struct {
		name                 string
		legacy, specFaithful bool
	}{
		{"jc", true, false},
		{"jc-v1.0.0", false, false},
		{"jc-v1.1.0", true, true},
	}

	for _, cfg := range configs {
		for _, v := range variants {
			for dname, data := range datasets {
				algo, _ := plakarJC(bytes.NewReader(nil), cfg, v.legacy, v.specFaithful)
				want := plakar.Chunks(algo, data, cfg)

				_, mk := plakarJC(bytes.NewReader(data), cfg, v.legacy, v.specFaithful)
				got := collect(t, mk())

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

// TestChunkerImplementsRollinghashChunker exercises the type through the
// rollinghash.Chunker interface and checks Sum/WindowSize semantics.
func TestChunkerImplementsRollinghashChunker(t *testing.T) {
	data := randData(200 * 1024)
	var c rollinghash.Chunker = jumpchunker.New(bytes.NewReader(data), gearhash64.New(), 4096, 512, 16384)

	if c.WindowSize() != 64 {
		t.Fatalf("WindowSize = %d, want 64", c.WindowSize())
	}
	tab := gearhash64.New().Table()
	gearDigest := func(b []byte) uint64 {
		var fp uint64
		for _, x := range b {
			fp = (fp << 1) + tab[x]
		}
		return fp
	}

	sawCD, sawForced := false, false
	for c.Next() {
		end := c.Offset() + len(c.Bytes())
		if c.ContentDefined() {
			sawCD = true
			continue
		}
		// A forced cut at max or the final chunk: Sum() is now the Gear
		// fingerprint of the 64 bytes ending at the cut, not 0 (0 only when
		// the cut is within 64 bytes of the start of the stream).
		sawForced = true
		var want uint64
		if end >= 64 {
			want = gearDigest(data[end-64 : end])
		}
		if c.Sum() != want {
			t.Fatalf("forced/final chunk ending at %d: Sum() = %#x, want %#x", end, c.Sum(), want)
		}
	}
	if err := c.Err(); err != nil {
		t.Fatal(err)
	}
	if !sawCD || !sawForced {
		t.Fatalf("test data did not exercise both cut kinds (cd=%v forced=%v)", sawCD, sawForced)
	}
}

func TestChunkerDeterminism(t *testing.T) {
	data := randData(200 * 1024)
	const normalSize, min, max = 4096, 512, 16384

	c := jumpchunker.New(bytes.NewReader(data), gearhash64.New(), normalSize, min, max)
	want := collect(t, c)

	c = jumpchunker.New(iotest.OneByteReader(bytes.NewReader(data)), gearhash64.New(), normalSize, min, max)
	got := collect(t, c)

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
	// Empty input: no chunks.
	c := jumpchunker.New(bytes.NewReader(nil), gearhash64.New(), 256, 1, 64)
	if c.Next() {
		t.Error("empty: expected no chunks")
	}
	if c.Bytes() != nil || c.ContentDefined() {
		t.Error("empty: expected zero-value accessors")
	}

	// A stream shorter than the 64-byte window still yields its bytes as one
	// final, non-content-defined chunk (matches rollinghash.Chunker as of
	// v4.3.3). Sum stays 0: no full window ends at the cut.
	subWindow := testData(63)
	c = jumpchunker.New(bytes.NewReader(subWindow), gearhash64.New(), 256, 64, 1024)
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

	// Exactly the window size: still one chunk of the whole input.
	exact := testData(64)
	c = jumpchunker.New(bytes.NewReader(exact), gearhash64.New(), 256, 64, 1024)
	if got := collect(t, c); len(got) != 1 || !bytes.Equal(got[0], exact) {
		t.Errorf("exactly-window: expected one chunk of the whole input, got %d chunks", len(got))
	}
}

func TestChunkerForcedCut(t *testing.T) {
	data := randData(128 * 1024)
	const normalSize, min, max = 512, 200, 4096

	c := jumpchunker.New(bytes.NewReader(data), gearhash64.New(), normalSize, min, max)
	chunks := collect(t, c)
	for i, ch := range chunks {
		if len(ch) > max {
			t.Fatalf("chunk %d exceeds max: %d > %d", i, len(ch), max)
		}
		if i != len(chunks)-1 && len(ch) < min {
			t.Fatalf("non-final chunk %d shorter than min: %d < %d", i, len(ch), min)
		}
	}
	if joined := bytes.Join(chunks, nil); !bytes.Equal(joined, data) {
		t.Fatalf("reassembled %d bytes, want %d", len(joined), len(data))
	}
}

func TestChunkerAccessorLifecycle(t *testing.T) {
	data := randData(200 * 1024)
	c := jumpchunker.New(bytes.NewReader(data), gearhash64.New(), 4096, 512, 16384)

	if c.Bytes() != nil || c.ContentDefined() {
		t.Error("expected zero-value accessors before first Next")
	}
	for c.Next() {
	}
	if err := c.Err(); err != nil {
		t.Fatal(err)
	}
	if c.Bytes() != nil || c.ContentDefined() {
		t.Error("expected zero-value accessors after Next returns false")
	}
	if c.Next() {
		t.Error("Next returned true after exhaustion")
	}
}

func TestChunkerError(t *testing.T) {
	boom := errors.New("boom")
	c := jumpchunker.New(iotest.ErrReader(boom), gearhash64.New(), 256, 1, 64)
	if c.Next() {
		t.Error("expected Next to fail on reader error")
	}
	if !errors.Is(c.Err(), boom) {
		t.Errorf("expected Err to be boom, got %v", c.Err())
	}
}

func TestChunkerReset(t *testing.T) {
	data := randData(200 * 1024)
	const normalSize, min, max = 4096, 512, 16384

	c := jumpchunker.New(bytes.NewReader(data), gearhash64.New(), normalSize, min, max)
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
	const normalSize, min, max = 8192, 2 << 10, 64 << 10

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	c := jumpchunker.New(nil, gearhash64.New(), normalSize, min, max)
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
