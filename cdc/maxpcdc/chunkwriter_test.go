package maxpcdc_test

import (
	"bytes"
	"testing"

	"github.com/chmduquesne/rollinghash/v4/cdc/maxpcdc"
)

func drainWriter(t *testing.T, w *maxpcdc.ChunkWriter, data []byte, step int) [][]byte {
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

func TestChunkWriterMatchesChunker(t *testing.T) {
	for _, cfg := range []struct{ w, max int }{
		{128, 4 * 1024},
		{46, 1024},
		{16, 64},
	} {
		for _, data := range [][]byte{randData(300 * 1024), structData(200 * 1024), randData(40), randData(70*1024 + 3)} {
			want := collect(t, maxpcdc.New(bytes.NewReader(data), cfg.w, cfg.max))
			for _, step := range []int{1, 7, 4096, 0} {
				w := maxpcdc.NewChunkWriter(cfg.w, cfg.max)
				got := drainWriter(t, w, data, step)
				if !sameChunks(got, want) {
					t.Fatalf("w=%d max=%d step=%d: got %d chunks, want %d", cfg.w, cfg.max, step, len(got), len(want))
				}
			}
		}
	}
}

func TestChunkWriterEmpty(t *testing.T) {
	w := maxpcdc.NewChunkWriter(16, 64)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if w.Next() {
		t.Error("empty: expected no chunks")
	}
}
