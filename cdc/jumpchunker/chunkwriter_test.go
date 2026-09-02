package jumpchunker_test

import (
	"bytes"
	"errors"
	"testing"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/jumpchunker"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
)

// writeChunks feeds data to a ChunkWriter in pieces of size step (0 = all at
// once), draining Next after every Write and after Close.
func writeChunks(t *testing.T, w *jumpchunker.ChunkWriter, data []byte, step int) [][]byte {
	t.Helper()
	var out [][]byte
	drain := func() {
		for w.Next() {
			out = append(out, append([]byte(nil), w.Bytes()...))
		}
	}
	if step <= 0 {
		step = len(data)
	}
	for off := 0; off < len(data); off += step {
		end := min(off+step, len(data))
		if _, err := w.Write(data[off:end]); err != nil {
			t.Fatalf("Write: %v", err)
		}
		drain()
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	drain()
	if err := w.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	return out
}

// TestChunkWriterEdgeCases mirrors TestChunkerEdgeCases for the push API: a
// stream shorter than the window is still emitted as one final,
// non-content-defined chunk (rollinghash.ChunkWriter behaves this way as of
// v4.3.3), and a truly empty stream yields nothing.
func TestChunkWriterEdgeCases(t *testing.T) {
	w := jumpchunker.NewChunkWriter(gearhash64.New(), 256, 64, 1024)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if w.Next() {
		t.Error("empty: expected no chunks")
	}

	subWindow := testData(63)
	w = jumpchunker.NewChunkWriter(gearhash64.New(), 256, 64, 1024)
	if _, err := w.Write(subWindow); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if !w.Next() {
		t.Fatal("sub-window: expected one chunk")
	}
	if !bytes.Equal(w.Bytes(), subWindow) || w.ContentDefined() || w.Sum() != 0 {
		t.Errorf("sub-window: chunk=%d bytes contentDefined=%v sum=%d, want %d bytes / false / 0",
			len(w.Bytes()), w.ContentDefined(), w.Sum(), len(subWindow))
	}
	if w.Next() {
		t.Error("sub-window: expected exactly one chunk")
	}
	if err := w.Err(); err != nil {
		t.Fatal(err)
	}
}

// TestChunkWriterMatchesChunker checks the push API produces the same chunks as
// the pull API for the same bytes, across write-piece sizes.
func TestChunkWriterMatchesChunker(t *testing.T) {
	for _, cfg := range []struct{ normal, min, max int }{
		{8 * 1024, 2 * 1024, 64 * 1024},
		{1024, 64, 8 * 1024},
	} {
		for _, data := range [][]byte{randData(300 * 1024), testData(200 * 1024), randData(40), randData(70*1024 + 3)} {
			want := collect(t, jumpchunker.New(bytes.NewReader(data), gearhash64.New(), cfg.normal, cfg.min, cfg.max))
			for _, step := range []int{1, 7, 4096, 0} {
				w := jumpchunker.NewChunkWriter(gearhash64.New(), cfg.normal, cfg.min, cfg.max)
				got := writeChunks(t, w, data, step)
				if len(got) != len(want) {
					t.Fatalf("%v step=%d: got %d chunks, want %d", cfg, step, len(got), len(want))
				}
				for i := range want {
					if !bytes.Equal(got[i], want[i]) {
						t.Fatalf("%v step=%d chunk %d differs", cfg, step, i)
					}
				}
			}
		}
	}
}

func TestChunkWriterErrClosedAndReset(t *testing.T) {
	data := randData(120 * 1024)
	w := jumpchunker.NewChunkWriter(gearhash64.New(), 4096, 512, 16384)
	first := writeChunks(t, w, data, 8192)

	if _, err := w.Write([]byte("x")); !errors.Is(err, rollinghash.ErrClosed) {
		t.Fatalf("Write after Close: err = %v, want ErrClosed", err)
	}

	w.Reset()
	second := writeChunks(t, w, data, 8192)
	if len(first) != len(second) {
		t.Fatalf("after Reset: %d chunks, first pass had %d", len(second), len(first))
	}
	for i := range first {
		if !bytes.Equal(first[i], second[i]) {
			t.Fatalf("chunk %d differs after Reset", i)
		}
	}
}

// TestChunkWriterImplementsRollinghash exercises the type through the
// rollinghash.ChunkWriter interface.
func TestChunkWriterImplementsRollinghash(t *testing.T) {
	var w rollinghash.ChunkWriter = jumpchunker.NewChunkWriter(gearhash64.New(), 4096, 512, 16384)
	if w.WindowSize() != 64 {
		t.Fatalf("WindowSize = %d, want 64", w.WindowSize())
	}
	if _, err := w.Write(randData(50 * 1024)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	n := 0
	for w.Next() {
		n++
	}
	if err := w.Err(); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no chunks emitted")
	}
}
