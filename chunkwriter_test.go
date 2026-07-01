package rollinghash_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/chmduquesne/rollinghash/v4"
)

// collectChunkWriterChunks drains a ChunkWriter after Close, checking that
// Next() returns chunks with the documented pre-/post-Close semantics.
func collectChunkWriterChunks(t *testing.T, cw rollinghash.ChunkWriter) (chunks [][]byte, contentDefined []bool) {
	t.Helper()
	for cw.Next() {
		chunks = append(chunks, append([]byte(nil), cw.Bytes()...))
		contentDefined = append(contentDefined, cw.ContentDefined())
	}
	if err := cw.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	return chunks, contentDefined
}

// writeChunksInPieces feeds data into cw in pieces of size writeSize (or all
// at once if writeSize <= 0), draining whatever Next() makes available
// after each Write, then Closes and drains the rest.
func writeChunksInPieces(t *testing.T, cw rollinghash.ChunkWriter, data []byte, writeSize int) (chunks [][]byte, contentDefined []bool) {
	t.Helper()
	if writeSize <= 0 {
		writeSize = len(data) + 1
	}
	for i := 0; i < len(data); i += writeSize {
		end := min(i+writeSize, len(data))
		if _, err := cw.Write(data[i:end]); err != nil {
			t.Fatalf("Write: %v", err)
		}
		for cw.Next() {
			chunks = append(chunks, append([]byte(nil), cw.Bytes()...))
			contentDefined = append(contentDefined, cw.ContentDefined())
		}
	}
	cw.Close()
	for cw.Next() {
		chunks = append(chunks, append([]byte(nil), cw.Bytes()...))
		contentDefined = append(contentDefined, cw.ContentDefined())
	}
	if err := cw.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	return chunks, contentDefined
}

// TestChunkWriter checks the ChunkWriter against the same reference used for
// Chunker, fed the whole input in one Write.
func TestChunkWriter(t *testing.T) {
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

			cw := rollinghash.NewChunkWriter(h.new(), window, cfg.mask, rollinghash.WithBoundaries(cfg.min, cfg.max))
			if _, err := cw.Write(data); err != nil {
				t.Fatalf("[%s] Write: %v", h.name, err)
			}
			cw.Close()
			gotChunks, gotContentDefined := collectChunkWriterChunks(t, cw)

			equalChunks(t, h.name, gotChunks, wantChunks)
			for i := range wantContentDefined {
				if gotContentDefined[i] != wantContentDefined[i] {
					t.Fatalf("[%s] chunk %d ContentDefined: got %v want %v", h.name, i, gotContentDefined[i], wantContentDefined[i])
				}
			}
			if joined := bytes.Join(gotChunks, nil); !bytes.Equal(joined, data) {
				t.Fatalf("[%s] reassembled %d bytes, want %d", h.name, len(joined), len(data))
			}
		}
	}
}

// TestChunkWriterFeedGranularity checks that the same input produces the
// same chunk sequence regardless of how it is chopped across Write calls,
// and regardless of exactly when Next() is drained relative to Write.
func TestChunkWriterFeedGranularity(t *testing.T) {
	data := testData(200 * 1024)
	const window = 48
	const mask, min, max = 0x3ff, 512, 16384

	for _, h := range allHashes {
		want, wantCD := refChunk(h.new(), data, window, mask, min, max)

		for _, ws := range []int{1, 7, window, window + 1, 4096, len(data)} {
			cw := rollinghash.NewChunkWriter(h.new(), window, mask, rollinghash.WithBoundaries(min, max))
			got, gotCD := writeChunksInPieces(t, cw, data, ws)
			equalChunks(t, h.name, got, want)
			for i := range wantCD {
				if gotCD[i] != wantCD[i] {
					t.Fatalf("[%s] writeSize=%d chunk %d ContentDefined: got %v want %v", h.name, ws, i, gotCD[i], wantCD[i])
				}
			}
		}
	}
}

// TestChunkWriterVsChunker cross-checks ChunkWriter against Chunker fed the
// same data through a bytes.Reader.
func TestChunkWriterVsChunker(t *testing.T) {
	data := testData(150 * 1024)
	const window = 48
	const mask, min, max = 0x1ff, 256, 8192

	for _, h := range allHashes {
		c := rollinghash.NewChunker(bytes.NewReader(data), h.new(), window, mask, rollinghash.WithBoundaries(min, max))
		want, wantCD := collectChunks(t, c)

		cw := rollinghash.NewChunkWriter(h.new(), window, mask, rollinghash.WithBoundaries(min, max))
		if _, err := cw.Write(data); err != nil {
			t.Fatal(err)
		}
		cw.Close()
		got, gotCD := collectChunkWriterChunks(t, cw)

		equalChunks(t, h.name, got, want)
		for i := range wantCD {
			if gotCD[i] != wantCD[i] {
				t.Fatalf("[%s] chunk %d ContentDefined: got %v want %v", h.name, i, gotCD[i], wantCD[i])
			}
		}
	}
}

// TestChunkWriterEdgeCases covers sub-window, exactly-window, and empty
// inputs, matching Chunker's documented behavior.
func TestChunkWriterEdgeCases(t *testing.T) {
	const window = 16

	for _, h := range allHashes {
		cw := rollinghash.NewChunkWriter(h.new(), window, 0xff, rollinghash.WithBoundaries(1, 64))
		if _, err := cw.Write(testData(window - 1)); err != nil {
			t.Fatal(err)
		}
		cw.Close()
		if cw.Next() {
			t.Errorf("[%s] sub-window: expected no chunks, got %d bytes", h.name, len(cw.Bytes()))
		}
		if cw.Bytes() != nil || cw.Sum() != 0 || cw.ContentDefined() {
			t.Errorf("[%s] sub-window: expected zero-value accessors", h.name)
		}

		data := testData(window)
		cw = rollinghash.NewChunkWriter(h.new(), window, 0xffffffff, rollinghash.WithBoundaries(1, 64))
		if _, err := cw.Write(data); err != nil {
			t.Fatal(err)
		}
		cw.Close()
		got, _ := collectChunkWriterChunks(t, cw)
		if len(got) != 1 || !bytes.Equal(got[0], data) {
			t.Errorf("[%s] exactly-window: expected one chunk of the whole input, got %d chunks", h.name, len(got))
		}

		cw = rollinghash.NewChunkWriter(h.new(), window, 0xff, rollinghash.WithBoundaries(1, 64))
		cw.Close()
		if cw.Next() {
			t.Errorf("[%s] empty: expected no chunks", h.name)
		}
	}
}

// TestChunkWriterPrePostClose verifies the documented Next() semantics: it
// returns false without emitting anything before Close when no boundary is
// available yet, and after Close it drains the remaining chunk(s) before
// settling to false for good.
func TestChunkWriterPrePostClose(t *testing.T) {
	const window = 16
	// mask that will essentially never hit, so no boundary appears until Close.
	cw := rollinghash.NewChunkWriter(allHashes[0].new(), window, ^uint64(0), rollinghash.WithBoundaries(1, 1<<30))

	if _, err := cw.Write(testData(window + 10)); err != nil {
		t.Fatal(err)
	}
	if cw.Next() {
		t.Fatal("expected Next() to return false pre-Close with no boundary and below max")
	}

	cw.Close()
	if !cw.Next() {
		t.Fatal("expected Next() to return true post-Close to flush the trailing chunk")
	}
	if len(cw.Bytes()) != window+10 {
		t.Fatalf("got trailing chunk of %d bytes, want %d", len(cw.Bytes()), window+10)
	}
	if cw.Next() {
		t.Fatal("expected Next() to return false once fully drained")
	}
	if err := cw.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
}

// TestChunkWriterCoalesces verifies that with a small WithBatchSize, a
// chunk becomes available before Close as soon as it's complete, while with
// the default (larger) batch size the same input is withheld until Close
// flushes the coalescing buffer.
func TestChunkWriterCoalesces(t *testing.T) {
	const window = 16
	// A chunk well within one boundary hit, forced at max so we don't
	// depend on the mask; comfortably below the default 16 KiB batch size.
	const mask, min, max = ^uint64(0), 1, 64
	data := testData(max)

	// Default WithBatchSize (16 KiB): far above len(data), so Write must
	// withhold everything until Close.
	cw := rollinghash.NewChunkWriter(allHashes[0].new(), window, mask, rollinghash.WithBoundaries(min, max))
	if _, err := cw.Write(data); err != nil {
		t.Fatal(err)
	}
	if cw.Next() {
		t.Fatal("expected Next() to return false pre-Close: input is far below the default batch size")
	}
	cw.Close()
	if !cw.Next() {
		t.Fatal("expected Close to flush the coalesced chunk")
	}

	// WithBatchSize(window), the smallest valid threshold: coalescing is
	// small enough that the completed chunk (forced at max) is visible
	// before Close. window is used here to test that boundary case
	// specifically, not as a throughput recommendation: it gives up most of
	// the throughput a batch size of just a few KiB would already recover
	// (see the ChunkWriter doc comment).
	cw = rollinghash.NewChunkWriter(allHashes[0].new(), window, mask, rollinghash.WithBoundaries(min, max), rollinghash.WithBatchSize(window))
	if _, err := cw.Write(data); err != nil {
		t.Fatal(err)
	}
	if !cw.Next() {
		t.Fatal("expected Next() to return true pre-Close with a small WithBatchSize")
	}
}

// TestChunkWriterErrClosed verifies that Write returns ErrClosed after
// Close, without silently dropping the write, and that Reset clears the
// closed state so the writer can be reused.
func TestChunkWriterErrClosed(t *testing.T) {
	const window = 16
	const mask, min, max = ^uint64(0), 1, 64
	cw := rollinghash.NewChunkWriter(allHashes[0].new(), window, mask, rollinghash.WithBoundaries(min, max), rollinghash.WithBatchSize(window))

	data := testData(max)
	if _, err := cw.Write(data); err != nil {
		t.Fatal(err)
	}
	cw.Close()

	if _, err := cw.Write(data); !errors.Is(err, rollinghash.ErrClosed) {
		t.Fatalf("Write after Close: got err %v, want ErrClosed", err)
	}

	// The write-after-close attempt must not have been silently absorbed:
	// draining now must show exactly the one pre-Close chunk, nothing more.
	got, _ := collectChunkWriterChunks(t, cw)
	if len(got) != 1 || len(got[0]) != max {
		t.Fatalf("got %d chunks (first len %d) after a rejected post-Close write, want 1 chunk of %d bytes", len(got), len(got[0]), max)
	}

	// Reset un-closes the writer.
	cw.Reset()
	if _, err := cw.Write(data); err != nil {
		t.Fatalf("Write after Reset: %v", err)
	}
	cw.Close()
	got, _ = collectChunkWriterChunks(t, cw)
	if len(got) != 1 || len(got[0]) != max {
		t.Fatalf("got %d chunks (first len %d) after Reset, want 1 chunk of %d bytes", len(got), len(got[0]), max)
	}
}

// BenchmarkChunkWriter measures steady-state chunking throughput via
// ChunkWriter, using the same window/mask/boundaries as BenchmarkChunker so
// the two are directly comparable. Data is fed in writeSize pieces with
// Next() drained after every Write, matching the intended usage pattern
// (draining immediately keeps the internal accumulator bounded; batching
// many Writes before draining measurably hurts throughput).
func BenchmarkChunkWriter(b *testing.B) {
	const window = 56
	data := testData(1 << 20)
	const boundMask, boundMin, boundMax = 0x1fff, 2 << 10, 64 << 10
	const writeSize = 4 << 10

	for _, h := range allHashes {
		b.Run(h.name+"/fused", func(b *testing.B) {
			cw := rollinghash.NewChunkWriter(h.new(), window, boundMask, rollinghash.WithBoundaries(boundMin, boundMax))

			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				cw.Reset()
				for i := 0; i < len(data); i += writeSize {
					end := min(i+writeSize, len(data))
					if _, err := cw.Write(data[i:end]); err != nil {
						b.Fatal(err)
					}
					for cw.Next() {
						_ = cw.Bytes()
					}
				}
				cw.Close()
				for cw.Next() {
					_ = cw.Bytes()
				}
				if cw.Err() != nil {
					b.Fatal(cw.Err())
				}
			}
		})
	}
}

// TestChunkWriterReset verifies that Reset lets one ChunkWriter process
// multiple streams, matching a fresh ChunkWriter for each.
func TestChunkWriterReset(t *testing.T) {
	const window = 48
	const mask, min, max = 0x3ff, 256, 8192
	data := testData(100 * 1024)
	want, wantCD := refChunk(allHashes[0].new(), data, window, mask, min, max)

	cw := rollinghash.NewChunkWriter(allHashes[0].new(), window, mask, rollinghash.WithBoundaries(min, max))
	for range 3 {
		cw.Reset()
		if _, err := cw.Write(data); err != nil {
			t.Fatal(err)
		}
		cw.Close()
		got, gotCD := collectChunkWriterChunks(t, cw)
		equalChunks(t, "reset", got, want)
		for i := range wantCD {
			if gotCD[i] != wantCD[i] {
				t.Fatalf("chunk %d ContentDefined: got %v want %v", i, gotCD[i], wantCD[i])
			}
		}
	}
}

// FuzzChunkWriter cross-checks ChunkWriter against the same reference used
// for Chunker, feeding the input in randomly sized Write calls.
func FuzzChunkWriter(f *testing.F) {
	f.Add([]byte("The quick brown fox jumps over the lazy dog"), 4, uint64(0x3), 6, 12, 5)
	f.Add(testData(9000), 16, uint64(0x1f), 40, 500, 37)

	f.Fuzz(func(t *testing.T, data []byte, window int, mask uint64, min, max, writeSize int) {
		if len(data) == 0 || window < 1 || window > len(data) {
			return
		}
		if min < window {
			min = window
		}
		if max < min {
			max = min
		}
		if max > 4*len(data)+window {
			max = 4*len(data) + window
		}
		if writeSize < 1 {
			writeSize = 1
		}

		for _, hc := range allHashes {
			want, wantCD := refChunk(hc.new(), data, window, mask, min, max)
			cw := rollinghash.NewChunkWriter(hc.new(), window, mask, rollinghash.WithBoundaries(min, max))
			got, gotCD := writeChunksInPieces(t, cw, data, writeSize)

			equalChunks(t, hc.name, got, want)
			for i := range wantCD {
				if i < len(gotCD) && gotCD[i] != wantCD[i] {
					t.Fatalf("[%s] chunk %d ContentDefined: got %v want %v", hc.name, i, gotCD[i], wantCD[i])
				}
			}
		}
	})
}
