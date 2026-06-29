package rollinghash_test

import (
	"bytes"
	"errors"
	"testing"
	"testing/iotest"

	"github.com/chmduquesne/rollinghash/v4"
)

// refChunk is an independent reference for the Chunker: it computes every window
// checksum once (via the classic hash) and applies the same boundary policy in
// a plain loop. The Chunker, however it derives its checksums, must agree.
func refChunk(classic interface {
	Reset()
	Write([]byte) (int, error)
	Sum([]byte) []byte
}, data []byte, window int, mask uint64, min, max int) (chunks [][]byte, contentDefined []bool) {
	sums := batchRollOracleHash(classic, data, window) // sums[g] = checksum of data[g:g+window]

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
			contentDefined = append(contentDefined, false)
			break
		}
		chunks = append(chunks, data[start:cut+1])
		contentDefined = append(contentDefined, hit)
		start = cut + 1
	}
	return chunks, contentDefined
}

func batchRollOracleHash(classic interface {
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

func collectChunks(t *testing.T, c rollinghash.Chunker) (chunks [][]byte, contentDefined []bool) {
	t.Helper()
	for c.Next() {
		chunks = append(chunks, append([]byte(nil), c.Bytes()...)) // copy: valid only until next Next
		contentDefined = append(contentDefined, c.ContentDefined())
	}
	if err := c.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	return chunks, contentDefined
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

// TestChunker checks the Chunker against the reference across several
// configurations, on data large enough to span many batches.
func TestChunker(t *testing.T) {
	data := testData(300 * 1024)
	const window = 56
	configs := []struct {
		mask     uint64
		min, max int
	}{
		{0x3ff, 256, 8192},
		{0xfff, 1024, 65536},
		{0x7f, 100, 1000},
	}

	for _, h := range allHashes {
		for _, cfg := range configs {
			wantChunks, wantContentDefined := refChunk(h.new(), data, window, cfg.mask, cfg.min, cfg.max)

			c := rollinghash.NewChunker(bytes.NewReader(data), h.new(), window, cfg.mask, rollinghash.WithBoundaries(cfg.min, cfg.max))
			gotChunks, gotContentDefined := collectChunks(t, c)

			equalChunks(t, h.name, gotChunks, wantChunks)
			for i := range wantContentDefined {
				if gotContentDefined[i] != wantContentDefined[i] {
					t.Fatalf("[%s] chunk %d ContentDefined: got %v want %v", h.name, i, gotContentDefined[i], wantContentDefined[i])
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


// TestChunkerContentDefined verifies ContentDefined/Sum: a mask boundary satisfies sum&mask==0,
// and a non-final forced boundary is exactly max bytes.
func TestChunkerContentDefined(t *testing.T) {
	data := testData(128 * 1024)
	const window = 56
	const mask, min, max = 0x1ff, 200, 4096

	for _, h := range allHashes {
		c := rollinghash.NewChunker(bytes.NewReader(data), h.new(), window, mask, rollinghash.WithBoundaries(min, max))
		var chunks [][]byte
		var sums []uint64
		var contentDefined []bool
		for c.Next() {
			chunks = append(chunks, append([]byte(nil), c.Bytes()...))
			sums = append(sums, c.Sum())
			contentDefined = append(contentDefined, c.ContentDefined())
		}
		for i := range chunks {
			if contentDefined[i] {
				if sums[i]&mask != 0 {
					t.Fatalf("[%s] chunk %d ContentDefined but Sum 0x%x & mask != 0", h.name, i, sums[i])
				}
			} else {
				if sums[i] != 0 {
					t.Fatalf("[%s] chunk %d forced cut but Sum 0x%x != 0", h.name, i, sums[i])
				}
				if i != len(chunks)-1 && len(chunks[i]) != max {
					t.Fatalf("[%s] chunk %d forced cut but length %d != max %d", h.name, i, len(chunks[i]), max)
				}
			}
		}
	}
}

// TestChunkerDeterminism feeds the same data through a one-byte-at-a-time reader
// (stressing the refill) and checks the chunking is identical.
func TestChunkerDeterminism(t *testing.T) {
	data := testData(200 * 1024)
	const window = 48
	const mask, min, max = 0x3ff, 512, 16384

	for _, h := range allHashes {
		base := rollinghash.NewChunker(bytes.NewReader(data), h.new(), window, mask, rollinghash.WithBoundaries(min, max))
		want, _ := collectChunks(t, base)

		slow := rollinghash.NewChunker(iotest.OneByteReader(bytes.NewReader(data)), h.new(), window, mask, rollinghash.WithBoundaries(min, max))
		got, _ := collectChunks(t, slow)

		equalChunks(t, h.name+"/onebyte", got, want)
	}
}

// TestChunkerEdgeCases covers sub-window, exactly-window, and empty inputs.
func TestChunkerEdgeCases(t *testing.T) {
	const window = 16

	for _, h := range allHashes {
		c := rollinghash.NewChunker(bytes.NewReader(testData(window-1)), h.new(), window, 0xff, rollinghash.WithBoundaries(1, 64))
		if c.Next() {
			t.Errorf("[%s] sub-window: expected no chunks, got %d bytes", h.name, len(c.Bytes()))
		}
		if c.Bytes() != nil || c.Sum() != 0 || c.ContentDefined() {
			t.Errorf("[%s] sub-window: expected zero-value accessors", h.name)
		}

		data := testData(window)
		c = rollinghash.NewChunker(bytes.NewReader(data), h.new(), window, 0xffffffff, rollinghash.WithBoundaries(1, 64))
		got, _ := collectChunks(t, c)
		if len(got) != 1 || !bytes.Equal(got[0], data) {
			t.Errorf("[%s] exactly-window: expected one chunk of the whole input, got %d chunks", h.name, len(got))
		}

		c = rollinghash.NewChunker(bytes.NewReader(nil), h.new(), window, 0xff, rollinghash.WithBoundaries(1, 64))
		if c.Next() {
			t.Errorf("[%s] empty: expected no chunks", h.name)
		}
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

		for _, hc := range allHashes {
			want, wantContentDefined := refChunk(hc.new(), data, window, mask, min, max)
			c := rollinghash.NewChunker(bytes.NewReader(data), hc.new(), window, mask, rollinghash.WithBoundaries(min, max))
			got, gotContentDefined := collectChunks(t, c)

			equalChunks(t, hc.name, got, want)
			for i := range wantContentDefined {
				if i < len(gotContentDefined) && gotContentDefined[i] != wantContentDefined[i] {
					t.Fatalf("[%s] chunk %d ContentDefined: got %v want %v", hc.name, i, gotContentDefined[i], wantContentDefined[i])
				}
			}
		}
	})
}

// TestChunkerAccessorLifecycle verifies that Chunk(), Sum(), and ContentDefined()
// return zero values both before the first Next() call and after Next()
// returns false on a stream that produced chunks.
func TestChunkerAccessorLifecycle(t *testing.T) {
	data := testData(200 * 1024)
	const window = 48
	const mask, min, max = 0x3ff, 512, 16384

	for _, h := range allHashes {
		c := rollinghash.NewChunker(bytes.NewReader(data), h.new(), window, mask, rollinghash.WithBoundaries(min, max))

		if c.Bytes() != nil || c.Sum() != 0 || c.ContentDefined() {
			t.Errorf("[%s] expected zero-value accessors before first Next", h.name)
		}

		for c.Next() {
		}
		if err := c.Err(); err != nil {
			t.Fatalf("[%s] Err: %v", h.name, err)
		}

		if c.Bytes() != nil || c.Sum() != 0 || c.ContentDefined() {
			t.Errorf("[%s] expected zero-value accessors after Next returns false", h.name)
		}

		// Calling Next() again after exhaustion must also return zero values.
		if c.Next() {
			t.Errorf("[%s] Next() returned true after exhaustion", h.name)
		}
		if c.Bytes() != nil || c.Sum() != 0 || c.ContentDefined() {
			t.Errorf("[%s] expected zero-value accessors on repeated Next after exhaustion", h.name)
		}
	}
}

// TestChunkerError verifies that a reader error is surfaced through Err.
func TestChunkerError(t *testing.T) {
	boom := errors.New("boom")
	for _, h := range allHashes {
		c := rollinghash.NewChunker(iotest.ErrReader(boom), h.new(), 16, 0xff, rollinghash.WithBoundaries(1, 64))
		if c.Next() {
			t.Errorf("[%s] expected Next to fail on reader error", h.name)
		}
		if !errors.Is(c.Err(), boom) {
			t.Errorf("[%s] expected Err to be boom, got %v", h.name, c.Err())
		}
	}
}

// BenchmarkChunker measures steady-state chunking throughput via BatchBoundaries
// across every hash in allHashes.
func BenchmarkChunker(b *testing.B) {
	const window = 56
	data := testData(1 << 20)
	const mask, min, max = 0x1fff, 2 << 10, 64 << 10

	for _, h := range allHashes {
		b.Run(h.name+"/fused", func(b *testing.B) {
			r := bytes.NewReader(data)
			ck := rollinghash.NewChunker(r, h.new(), window, mask, rollinghash.WithBoundaries(min, max))

			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				r.Reset(data)
				ck.Reset(r)
				for ck.Next() {
					_ = ck.Bytes()
				}
				if ck.Err() != nil {
					b.Fatal(ck.Err())
				}
			}
		})
	}
}
