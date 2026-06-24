package rollinghash_test

import (
	"bytes"
	"errors"
	"testing"
	"testing/iotest"

	"github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/adler32"
	"github.com/chmduquesne/rollinghash/v4/bozo64"
)

// refChunk is an independent reference for the Chunker: it computes every window
// checksum once (via the classic hash) and applies the same boundary policy in
// a plain loop. The Chunker, however it derives its checksums, must agree.
func refChunk(classic interface {
	Reset()
	Write([]byte) (int, error)
	Sum([]byte) []byte
}, data []byte, window int, mask uint64, min, max int) (chunks [][]byte, atMask []bool) {
	sums := bulkRollOracleHash(classic, data, window) // sums[g] = checksum of data[g:g+window]

	start := 0
	for start < len(data) {
		cut, hit := -1, false
		for L := 1; start+L-1 < len(data); L++ {
			e := start + L - 1  // candidate boundary byte
			g := e - window + 1 // sum index of the window ending at e
			if L >= min && g >= 0 && sums[g]&mask == 0 {
				cut, hit = e, true
				break
			}
			if L >= max && g >= 0 {
				cut, hit = e, false
				break
			}
		}
		if cut < 0 {
			chunks = append(chunks, data[start:])
			atMask = append(atMask, false)
			break
		}
		chunks = append(chunks, data[start:cut+1])
		atMask = append(atMask, hit)
		start = cut + 1
	}
	return chunks, atMask
}

func bulkRollOracleHash(classic interface {
	Reset()
	Write([]byte) (int, error)
	Sum([]byte) []byte
}, data []byte, window int) []uint64 {
	if window <= 0 || len(data) < window {
		return nil
	}
	out := make([]uint64, len(data)-window+1)
	for i := range out {
		classic.Reset()
		if _, err := classic.Write(data[i : i+window]); err != nil {
			panic(err)
		}
		var v uint64
		for _, b := range classic.Sum(make([]byte, 0, 8)) {
			v = v<<8 | uint64(b)
		}
		out[i] = v
	}
	return out
}

func collectChunks(t *testing.T, c *rollinghash.Chunker) (chunks [][]byte, atMask []bool) {
	t.Helper()
	for c.Next() {
		chunks = append(chunks, append([]byte(nil), c.Chunk()...)) // copy: valid only until next Next
		atMask = append(atMask, c.AtMask())
	}
	if err := c.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	return chunks, atMask
}

func equalChunks(t *testing.T, name string, got, want [][]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("[%s] got %d chunks, want %d", name, len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("[%s] chunk %d: got %d bytes, want %d bytes", name, i, len(got[i]), len(want[i]))
		}
	}
}

var chunkerHashes = []struct {
	name string
	new  func() rollinghash.Hash
}{
	{"bozo64", func() rollinghash.Hash { return bozo64.New() }},
	{"adler32", func() rollinghash.Hash { return adler32.New() }},
}

// TestChunker checks the Chunker against the reference across several
// configurations, on data large enough to span many batches.
func TestChunker(t *testing.T) {
	data := testData(300 * 1024)
	const window = 64
	configs := []struct {
		mask     uint64
		min, max int
	}{
		{0x3ff, 256, 8192},
		{0xfff, 1024, 65536},
		{0x7f, 100, 1000},
	}

	for _, h := range chunkerHashes {
		for _, cfg := range configs {
			wantChunks, wantAtMask := refChunk(h.new(), data, window, cfg.mask, cfg.min, cfg.max)

			c := rollinghash.NewChunker(bytes.NewReader(data), h.new(), window, cfg.mask, cfg.min, cfg.max)
			gotChunks, gotAtMask := collectChunks(t, c)

			equalChunks(t, h.name, gotChunks, wantChunks)
			for i := range wantAtMask {
				if gotAtMask[i] != wantAtMask[i] {
					t.Fatalf("[%s] chunk %d AtMask: got %v want %v", h.name, i, gotAtMask[i], wantAtMask[i])
				}
			}
			if joined := bytes.Join(gotChunks, nil); !bytes.Equal(joined, data) {
				t.Fatalf("[%s] reassembled %d bytes, want %d", h.name, len(joined), len(data))
			}
			for i, ch := range gotChunks[:len(gotChunks)-1] {
				if len(ch) < cfg.min || len(ch) > cfg.max {
					t.Fatalf("[%s] chunk %d length %d outside [%d,%d]", h.name, i, len(ch), cfg.min, cfg.max)
				}
			}
		}
	}
}

// bulkOnly forwards BulkRoll but hides BulkBoundaries, forcing the Chunker's
// BulkRoll fallback. rollOnly hides both, forcing the Write+Roll fallback.
type bulkOnly struct{ rollinghash.Hash }

func (b bulkOnly) BulkRoll(dst []uint64, data []byte, window int) {
	b.Hash.(rollinghash.BulkRoller).BulkRoll(dst, data, window)
}

type rollOnly struct{ rollinghash.Hash }

// TestChunkerPaths checks that the fused, BulkRoll-fallback, and Roll-fallback
// paths all produce byte-identical chunks.
func TestChunkerPaths(t *testing.T) {
	data := testData(200 * 1024)
	const window = 48
	const mask, min, max = 0x3ff, 300, 20000

	fused := rollinghash.NewChunker(bytes.NewReader(data), bozo64.New(), window, mask, min, max)
	want, wantMask := collectChunks(t, fused)

	for _, p := range []struct {
		name string
		h    rollinghash.Hash
	}{
		{"bulkFallback", bulkOnly{bozo64.New()}},
		{"rollFallback", rollOnly{bozo64.New()}},
	} {
		c := rollinghash.NewChunker(bytes.NewReader(data), p.h, window, mask, min, max)
		got, gotMask := collectChunks(t, c)
		equalChunks(t, p.name, got, want)
		for i := range wantMask {
			if gotMask[i] != wantMask[i] {
				t.Fatalf("[%s] chunk %d AtMask mismatch", p.name, i)
			}
		}
	}
}

// TestChunkerAtMask verifies AtMask/Sum: a mask boundary satisfies sum&mask==0,
// and a non-final forced boundary is exactly max bytes.
func TestChunkerAtMask(t *testing.T) {
	data := testData(128 * 1024)
	const window = 64
	const mask, min, max = 0x1ff, 200, 4096

	c := rollinghash.NewChunker(bytes.NewReader(data), bozo64.New(), window, mask, min, max)
	var chunks [][]byte
	var idx int
	var sums []uint64
	var atMask []bool
	for c.Next() {
		chunks = append(chunks, append([]byte(nil), c.Chunk()...))
		sums = append(sums, c.Sum())
		atMask = append(atMask, c.AtMask())
		idx++
	}
	for i := range chunks {
		if atMask[i] {
			if sums[i]&mask != 0 {
				t.Fatalf("chunk %d AtMask but Sum 0x%x & mask != 0", i, sums[i])
			}
		} else if i != len(chunks)-1 && len(chunks[i]) != max {
			t.Fatalf("chunk %d forced cut but length %d != max %d", i, len(chunks[i]), max)
		}
	}
}

// TestChunkerDeterminism feeds the same data through a one-byte-at-a-time reader
// (stressing the refill) and checks the chunking is identical.
func TestChunkerDeterminism(t *testing.T) {
	data := testData(200 * 1024)
	const window = 48
	const mask, min, max = 0x3ff, 512, 16384

	base := rollinghash.NewChunker(bytes.NewReader(data), bozo64.New(), window, mask, min, max)
	want, _ := collectChunks(t, base)

	slow := rollinghash.NewChunker(iotest.OneByteReader(bytes.NewReader(data)), bozo64.New(), window, mask, min, max)
	got, _ := collectChunks(t, slow)

	equalChunks(t, "onebyte", got, want)
}

// TestChunkerEdgeCases covers sub-window, exactly-window, and empty inputs.
func TestChunkerEdgeCases(t *testing.T) {
	const window = 16

	c := rollinghash.NewChunker(bytes.NewReader(testData(window-1)), bozo64.New(), window, 0xff, 1, 64)
	if c.Next() {
		t.Errorf("sub-window: expected no chunks, got %d bytes", len(c.Chunk()))
	}
	if c.Chunk() != nil || c.Sum() != 0 || c.AtMask() {
		t.Errorf("sub-window: expected zero-value accessors")
	}

	data := testData(window)
	c = rollinghash.NewChunker(bytes.NewReader(data), bozo64.New(), window, 0xffffffff, 1, 64)
	got, _ := collectChunks(t, c)
	if len(got) != 1 || !bytes.Equal(got[0], data) {
		t.Errorf("exactly-window: expected one chunk of the whole input, got %d chunks", len(got))
	}

	c = rollinghash.NewChunker(bytes.NewReader(nil), bozo64.New(), window, 0xff, 1, 64)
	if c.Next() {
		t.Errorf("empty: expected no chunks")
	}
}

// FuzzChunker cross-checks the Chunker against the reference on random data
// and parameters (kept to window <= min <= max so the two agree).
func FuzzChunker(f *testing.F) {
	f.Add([]byte("The quick brown fox jumps over the lazy dog"), 4, uint64(0x3), 6, 12)
	f.Add(testData(9000), 16, uint64(0x1f), 40, 500)

	f.Fuzz(func(t *testing.T, data []byte, window int, mask uint64, min, max int) {
		if len(data) == 0 || window < 1 || window > len(data) {
			return
		}
		if min < window {
			min = window
		}
		if max < min {
			max = min
		}
		if max > 4*len(data)+window { // keep it bounded
			max = 4*len(data) + window
		}

		want, wantMask := refChunk(bozo64.New(), data, window, mask, min, max)
		c := rollinghash.NewChunker(bytes.NewReader(data), bozo64.New(), window, mask, min, max)
		got, gotMask := collectChunks(t, c)

		equalChunks(t, "fuzz", got, want)
		for i := range wantMask {
			if i < len(gotMask) && gotMask[i] != wantMask[i] {
				t.Fatalf("chunk %d AtMask: got %v want %v", i, gotMask[i], wantMask[i])
			}
		}
	})
}

// TestChunkerError verifies that a reader error is surfaced through Err.
func TestChunkerError(t *testing.T) {
	boom := errors.New("boom")
	c := rollinghash.NewChunker(iotest.ErrReader(boom), bozo64.New(), 16, 0xff, 1, 64)
	if c.Next() {
		t.Errorf("expected Next to fail on reader error")
	}
	if !errors.Is(c.Err(), boom) {
		t.Errorf("expected Err to be boom, got %v", c.Err())
	}
}

func BenchmarkChunker(b *testing.B) {
	const window = 64
	data := testData(1 << 20)
	const mask, min, max = 0x1fff, 2 << 10, 64 << 10

	c := rollinghash.NewChunker(bytes.NewReader(data), bozo64.New(), window, mask, min, max)
	r := bytes.NewReader(data)

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.Reset(data)
		c.Reset(r)
		for c.Next() {
			_ = c.Chunk()
		}
		if c.Err() != nil {
			b.Fatal(c.Err())
		}
	}
}
