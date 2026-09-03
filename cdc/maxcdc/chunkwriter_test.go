package maxcdc_test

import (
	"bytes"
	"testing"

	"github.com/chmduquesne/rollinghash/v4/cdc/maxcdc"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
)

func drainWriter(t *testing.T, w *maxcdc.ChunkWriter, data []byte, step int) [][]byte {
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
	for _, cfg := range []struct{ min, max int }{
		{512, 4 * 1024},
		{128, 1024},
		{1024, 2048},
	} {
		for _, data := range [][]byte{randData(300 * 1024), structData(200 * 1024), randData(40), randData(70*1024 + 3)} {
			want := collect(t, maxcdc.New(bytes.NewReader(data), gearhash64.New(), cfg.min, cfg.max))
			for _, step := range []int{1, 7, 4096, 0} {
				w := maxcdc.NewChunkWriter(gearhash64.New(), cfg.min, cfg.max)
				got := drainWriter(t, w, data, step)
				if !sameChunks(got, want) {
					t.Fatalf("min=%d max=%d step=%d: got %d chunks, want %d", cfg.min, cfg.max, step, len(got), len(want))
				}
			}
		}
	}
}

func TestChunkWriterEmpty(t *testing.T) {
	w := maxcdc.NewChunkWriter(gearhash64.New(), 256, 512)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if w.Next() {
		t.Error("empty: expected no chunks")
	}
}
