package ultracdc_test

import (
	"bytes"
	"testing"

	"github.com/chmduquesne/rollinghash/v4/cdc/ultracdc"
)

func drainWriter(t *testing.T, w *ultracdc.ChunkWriter, data []byte, step int) [][]byte {
	t.Helper()
	var out [][]byte
	pump := func() {
		for w.Next() {
			out = append(out, append([]byte(nil), w.Bytes()...))
		}
	}
	if step <= 0 {
		step = len(data)
	}
	for off := 0; off < len(data); off += step {
		if _, err := w.Write(data[off:min(off+step, len(data))]); err != nil {
			t.Fatalf("Write: %v", err)
		}
		pump()
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	pump()
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
	w := ultracdc.NewChunkWriter(64, 1024, 8192)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if w.Next() {
		t.Error("empty: expected no chunks")
	}

	subWindow := randData(7)
	w = ultracdc.NewChunkWriter(64, 1024, 8192)
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
	for _, cfg := range []struct{ min, normal, max int }{
		{2 * 1024, 10 * 1024, 64 * 1024},
		{128, 1024, 8 * 1024},
	} {
		for _, data := range [][]byte{randData(300 * 1024), lowEntropyData(300 * 1024), randData(40), randData(70*1024 + 3)} {
			want := collect(t, ultracdc.New(bytes.NewReader(data), cfg.min, cfg.normal, cfg.max))
			for _, step := range []int{1, 7, 4096, 0} {
				w := ultracdc.NewChunkWriter(cfg.min, cfg.normal, cfg.max)
				got := drainWriter(t, w, data, step)
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
