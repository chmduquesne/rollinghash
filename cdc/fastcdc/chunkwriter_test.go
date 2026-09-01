package fastcdc_test

import (
	"bytes"
	"testing"

	"github.com/chmduquesne/rollinghash/v4/cdc/fastcdc"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
)

func drainWriter(t *testing.T, w *fastcdc.ChunkWriter, data []byte, step int) [][]byte {
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

// TestChunkWriterMatchesChunker checks the push API produces the same chunks as
// the pull API for the same bytes, across write-piece sizes.
func TestChunkWriterMatchesChunker(t *testing.T) {
	for _, cfg := range []struct{ min, normal, max int }{
		{2 * 1024, 8 * 1024, 64 * 1024},
		{512, 4 * 1024, 32 * 1024},
	} {
		for _, data := range [][]byte{randData(300 * 1024), structData(200 * 1024), randData(40), randData(70*1024 + 3)} {
			want := collect(t, fastcdc.New(bytes.NewReader(data), gearhash64.New(), cfg.min, cfg.normal, cfg.max))
			for _, step := range []int{1, 7, 4096, 0} {
				w := fastcdc.NewChunkWriter(gearhash64.New(), cfg.min, cfg.normal, cfg.max)
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
