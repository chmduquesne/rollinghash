package rollinghash_test

import (
	"bytes"
	"errors"
	"math/bits"
	"testing"
	"testing/iotest"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
)

// jumpRoller mirrors the unexported jumpBoundaryRoller interface so the test
// package can call JumpBoundaries without importing internal types.
type jumpRoller interface {
	JumpBoundaries(a []int32, data []byte, maskC uint64, jumpLen int, fp uint64, firstSkip, minStep int) (n int, newFp uint64, skip int)
}

// jumpTestParams mirrors the internal jumpParams derivation so the reference
// implementation uses the same maskC/jumpLen as JumpChunker.
func jumpTestParams(normalSize int) (maskC uint64, jumpLen int) {
	lg := bits.Len(uint(normalSize)) - 1
	if lg < 3 {
		lg = 3
	}
	cOnes := lg - 2
	jumpLen = 1 << (lg - 1)
	step := 64 / cOnes
	for i := 0; i < cOnes; i++ {
		maskC |= 1 << uint(63-i*step)
	}
	return
}

// refJumpChunk is the reference implementation of Jump Chunking.
// It calls JumpBoundaries once per chunk (always with fp=0), matching the
// semantics of JumpChunker which resets fp at the start of every chunk's scan
// region — including after forced cuts at max. A single-pass call would fail
// to reset fp at forced cuts and would diverge.
func refJumpChunk(jr jumpRoller, data []byte, normalSize, min, max int) ([][]byte, []bool) {
	if len(data) == 0 {
		return nil, nil
	}
	maskC, jumpLen := jumpTestParams(normalSize)
	a := make([]int32, 2)

	var chunks [][]byte
	var atMask []bool

	start := 0
	for start < len(data) {
		maxByte := start + max - 1

		// Find the first JC boundary in data[start:] with fp=0. JumpBoundaries
		// skips the min zone (firstSkip=min) and resets fp there, exactly as
		// JumpChunker does at each chunk boundary.
		slice := data[start:]
		nb, _, _ := jr.JumpBoundaries(a[:1], slice, maskC, jumpLen, 0, min, min)

		var e int
		var hit bool
		if nb > 0 && start+int(a[0]) <= maxByte {
			e = start + int(a[0])
			hit = true
		}

		switch {
		case hit:
			chunks = append(chunks, data[start:e+1])
			atMask = append(atMask, true)
			start = e + 1
		case maxByte <= len(data)-1:
			chunks = append(chunks, data[start:maxByte+1])
			atMask = append(atMask, false)
			start = maxByte + 1
		default:
			chunks = append(chunks, data[start:])
			atMask = append(atMask, false)
			start = len(data)
		}
	}
	return chunks, atMask
}

func collectJumpChunks(t *testing.T, c *rollinghash.JumpChunker) ([][]byte, []bool) {
	t.Helper()
	var chunks [][]byte
	var atMask []bool
	for c.Next() {
		chunks = append(chunks, append([]byte(nil), c.Bytes()...))
		atMask = append(atMask, c.AtMask())
	}
	if err := c.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	return chunks, atMask
}

// TestJumpChunker checks the JumpChunker against the single-pass reference
// across several normalSize/min/max configurations.
func TestJumpChunker(t *testing.T) {
	data := testData(300 * 1024)
	configs := []struct {
		normalSize, min, max int
	}{
		{256, 64, 2048},
		{512, 256, 8192},
		{1024, 512, 16384},
		{128, 50, 800},
	}

	h := gearhash64.New()
	for _, cfg := range configs {
		want, wantMask := refJumpChunk(h, data, cfg.normalSize, cfg.min, cfg.max)

		c := rollinghash.NewJumpChunker(bytes.NewReader(data), gearhash64.New(), cfg.normalSize, cfg.min, cfg.max)
		got, gotMask := collectJumpChunks(t, c)

		if len(got) != len(want) {
			t.Fatalf("normalSize=%d: got %d chunks, want %d", cfg.normalSize, len(got), len(want))
		}
		for i := range want {
			if !bytes.Equal(got[i], want[i]) {
				t.Fatalf("normalSize=%d chunk %d: got %d bytes, want %d bytes", cfg.normalSize, i, len(got[i]), len(want[i]))
			}
			if gotMask[i] != wantMask[i] {
				t.Fatalf("normalSize=%d chunk %d: AtMask got %v want %v", cfg.normalSize, i, gotMask[i], wantMask[i])
			}
		}
		if joined := bytes.Join(got, nil); !bytes.Equal(joined, data) {
			t.Fatalf("normalSize=%d: reassembled %d bytes, want %d", cfg.normalSize, len(joined), len(data))
		}
	}
}

// TestJumpChunkerDeterminism verifies that a one-byte-at-a-time reader
// produces the same chunks as a normal reader.
func TestJumpChunkerDeterminism(t *testing.T) {
	data := testData(200 * 1024)
	const normalSize, min, max = 1024, 512, 16384

	c := rollinghash.NewJumpChunker(bytes.NewReader(data), gearhash64.New(), normalSize, min, max)
	want, _ := collectJumpChunks(t, c)

	c = rollinghash.NewJumpChunker(iotest.OneByteReader(bytes.NewReader(data)), gearhash64.New(), normalSize, min, max)
	got, _ := collectJumpChunks(t, c)

	if len(got) != len(want) {
		t.Fatalf("onebyte: got %d chunks, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("onebyte chunk %d: got %d bytes, want %d bytes", i, len(got[i]), len(want[i]))
		}
	}
}

// TestJumpChunkerEdgeCases covers empty, smaller-than-min, and exactly-min inputs.
func TestJumpChunkerEdgeCases(t *testing.T) {
	const normalSize = 256
	_, jumpLen := jumpTestParams(normalSize)

	// Empty input: no chunks.
	c := rollinghash.NewJumpChunker(bytes.NewReader(nil), gearhash64.New(), normalSize, 1, 64)
	if c.Next() {
		t.Error("empty: expected no chunks")
	}
	if c.Bytes() != nil || c.AtMask() {
		t.Error("empty: expected zero-value accessors")
	}

	// Input shorter than min: one final chunk.
	data := testData(10)
	c = rollinghash.NewJumpChunker(bytes.NewReader(data), gearhash64.New(), normalSize, 20, 1024)
	got, _ := collectJumpChunks(t, c)
	if len(got) != 1 || !bytes.Equal(got[0], data) {
		t.Errorf("short: expected one chunk of all data, got %d chunks", len(got))
	}

	// Input spanning at least one jump: check against reference.
	h := gearhash64.New()
	data = testData(jumpLen * 3)
	want, wantMask := refJumpChunk(h, data, normalSize, 64, 2048)
	c = rollinghash.NewJumpChunker(bytes.NewReader(data), gearhash64.New(), normalSize, 64, 2048)
	gotChunks, gotMask := collectJumpChunks(t, c)
	if len(gotChunks) != len(want) {
		t.Fatalf("jumpspan: got %d chunks, want %d", len(gotChunks), len(want))
	}
	for i := range want {
		if !bytes.Equal(gotChunks[i], want[i]) || gotMask[i] != wantMask[i] {
			t.Fatalf("jumpspan chunk %d mismatch", i)
		}
	}
}

// TestJumpChunkerAtMask verifies that forced cuts are exactly max bytes long
// (except for the final chunk).
func TestJumpChunkerAtMask(t *testing.T) {
	data := testData(128 * 1024)
	const normalSize, min, max = 512, 200, 4096

	c := rollinghash.NewJumpChunker(bytes.NewReader(data), gearhash64.New(), normalSize, min, max)
	var chunks [][]byte
	var atMask []bool
	for c.Next() {
		chunks = append(chunks, append([]byte(nil), c.Bytes()...))
		atMask = append(atMask, c.AtMask())
	}
	if err := c.Err(); err != nil {
		t.Fatal(err)
	}
	for i, ch := range chunks {
		if !atMask[i] && i != len(chunks)-1 && len(ch) != max {
			t.Errorf("chunk %d: forced cut but length %d != max %d", i, len(ch), max)
		}
	}
	// Chunks reassemble to the original data.
	if joined := bytes.Join(chunks, nil); !bytes.Equal(joined, data) {
		t.Fatalf("reassembled %d bytes, want %d", len(joined), len(data))
	}
}

// TestJumpChunkerAccessorLifecycle checks that Bytes() and AtMask() are nil/false
// before the first Next() and after Next() returns false.
func TestJumpChunkerAccessorLifecycle(t *testing.T) {
	data := testData(200 * 1024)
	const normalSize, min, max = 1024, 512, 16384

	c := rollinghash.NewJumpChunker(bytes.NewReader(data), gearhash64.New(), normalSize, min, max)

	if c.Bytes() != nil || c.AtMask() {
		t.Error("expected zero-value accessors before first Next")
	}
	for c.Next() {
	}
	if err := c.Err(); err != nil {
		t.Fatal(err)
	}
	if c.Bytes() != nil || c.AtMask() {
		t.Error("expected zero-value accessors after Next returns false")
	}
	if c.Next() {
		t.Error("Next returned true after exhaustion")
	}
	if c.Bytes() != nil || c.AtMask() {
		t.Error("expected zero-value accessors on repeated Next after exhaustion")
	}
}

// TestJumpChunkerError verifies that reader errors are surfaced via Err.
func TestJumpChunkerError(t *testing.T) {
	boom := errors.New("boom")
	c := rollinghash.NewJumpChunker(iotest.ErrReader(boom), gearhash64.New(), 256, 1, 64)
	if c.Next() {
		t.Error("expected Next to fail on reader error")
	}
	if !errors.Is(c.Err(), boom) {
		t.Errorf("expected Err to be boom, got %v", c.Err())
	}
}

// TestJumpChunkerReset checks that Reset reuses buffers correctly.
func TestJumpChunkerReset(t *testing.T) {
	data := testData(200 * 1024)
	const normalSize, min, max = 1024, 512, 16384

	h := gearhash64.New()
	want, _ := refJumpChunk(h, data, normalSize, min, max)

	c := rollinghash.NewJumpChunker(bytes.NewReader(data), gearhash64.New(), normalSize, min, max)
	// First pass.
	got1, _ := collectJumpChunks(t, c)
	// Second pass via Reset.
	c.Reset(bytes.NewReader(data))
	got2, _ := collectJumpChunks(t, c)

	for _, got := range [][][]byte{got1, got2} {
		if len(got) != len(want) {
			t.Fatalf("got %d chunks, want %d", len(got), len(want))
		}
		for i := range want {
			if !bytes.Equal(got[i], want[i]) {
				t.Fatalf("chunk %d mismatch after Reset", i)
			}
		}
	}
}

// benchRandData fills a slice with pseudo-random bytes using a fast
// xorshift64 PRNG. The deterministic testData formula happens to produce no
// natural JC boundaries for typical maskC values (a degenerate case), so we
// use random bytes here to get a realistic mix of scan and jump steps.
func benchRandData(n int) []byte {
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

// BenchmarkJumpChunker measures JumpChunker throughput and compares it to
// BenchmarkChunker/gearhash64/fused.
func BenchmarkJumpChunker(b *testing.B) {
	data := benchRandData(1 << 20)
	const normalSize, min, max = 8192, 2 << 10, 64 << 10

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	c := rollinghash.NewJumpChunker(nil, gearhash64.New(), normalSize, min, max)
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

