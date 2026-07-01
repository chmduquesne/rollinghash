package rollinghash_test

import (
	"bytes"
	"testing"

	"github.com/chmduquesne/rollinghash/v4"
)

// collectBatchWriterSums drains a BatchWriter after each Write and after
// Close, checking the per-batch alignment invariant along the way, and
// returns the concatenation of every batch's Sums().
func collectBatchWriterSums(t *testing.T, name string, w rollinghash.BatchWriter, window int) []uint64 {
	t.Helper()
	var got []uint64
	for w.Next() {
		sums, data := w.Sums(), w.Bytes()
		if len(sums) != len(data)-window+1 {
			t.Fatalf("[%s] alignment: len(Sums)=%d len(Bytes)=%d window=%d",
				name, len(sums), len(data), window)
		}
		got = append(got, sums...)
	}
	if err := w.Err(); err != nil {
		t.Fatalf("[%s] Err: %v", name, err)
	}
	return got
}

// writeInChunks feeds data into w in pieces of size chunkSize (or all at
// once if chunkSize <= 0), draining whatever Next() makes available after
// each Write, then Closes and drains the rest.
func writeInChunks(t *testing.T, w rollinghash.BatchWriter, data []byte, window, chunkSize int) []uint64 {
	t.Helper()
	var got []uint64
	if chunkSize <= 0 {
		chunkSize = len(data) + 1
	}
	for i := 0; i < len(data); i += chunkSize {
		end := min(i+chunkSize, len(data))
		if _, err := w.Write(data[i:end]); err != nil {
			t.Fatalf("Write: %v", err)
		}
		for w.Next() {
			sums, buf := w.Sums(), w.Bytes()
			if len(sums) != len(buf)-window+1 {
				t.Fatalf("alignment: len(Sums)=%d len(Bytes)=%d window=%d", len(sums), len(buf), window)
			}
			got = append(got, sums...)
		}
	}
	w.Close()
	for w.Next() {
		sums, buf := w.Sums(), w.Bytes()
		if len(sums) != len(buf)-window+1 {
			t.Fatalf("alignment: len(Sums)=%d len(Bytes)=%d window=%d", len(sums), len(buf), window)
		}
		got = append(got, sums...)
	}
	if err := w.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	return got
}

// TestBatchWriter checks that draining a BatchWriter fed the whole input in
// one Write matches the classic per-window hash.
func TestBatchWriter(t *testing.T) {
	data := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 5000)
	const window = 56
	for _, h := range allHashes {
		want := batchRollOracle(h.new(), data, window)
		w := rollinghash.NewBatchWriter(h.new(), window)
		if _, err := w.Write(data); err != nil {
			t.Fatalf("[%s] Write: %v", h.name, err)
		}
		w.Close()
		got := collectBatchWriterSums(t, h.name, w, window)
		equalSums(t, h.name, got, want)
	}
}

// TestBatchWriterFeedGranularity checks that the same input produces the
// same concatenated Sums() regardless of how it is chopped across Write
// calls, and regardless of the configured batch size.
func TestBatchWriterFeedGranularity(t *testing.T) {
	data := make([]byte, 500)
	for i := range data {
		data[i] = byte(i*73 + 11)
	}
	const window = 16
	writeSizes := []int{1, 7, window, window + 1, 97, len(data)}
	bufSizes := []int{window, window + 1, 2 * window, 3*window - 1, 97, len(data)}

	for _, h := range allHashes {
		want := batchRollOracle(h.new(), data, window)
		for _, ws := range writeSizes {
			for _, bs := range bufSizes {
				w := rollinghash.NewBatchWriter(h.new(), window, rollinghash.WithBufferSize(bs))
				got := writeInChunks(t, w, data, window, ws)
				equalSums(t, h.name, got, want)
			}
		}
	}
}

// TestBatchWriterBytes verifies that each batch's Bytes() holds the exact
// source bytes for that batch, across write/batch-size granularities.
func TestBatchWriterBytes(t *testing.T) {
	data := testData(5000)
	const window = 16
	w := rollinghash.NewBatchWriter(allHashes[0].new(), window, rollinghash.WithBufferSize(64))
	off := 0
	if _, err := w.Write(data[:1000]); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data[1000:]); err != nil {
		t.Fatal(err)
	}
	w.Close()
	for w.Next() {
		b := w.Bytes()
		if !bytes.Equal(b, data[off:off+len(b)]) {
			t.Fatalf("batch at offset %d does not match source", off)
		}
		off += len(b) - (window - 1)
	}
	if err := w.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
}

// TestBatchWriterShortInput verifies that an input shorter than window
// yields no batches (even after Close) and zero-value accessors, while
// exactly window bytes yields one sum.
func TestBatchWriterShortInput(t *testing.T) {
	const window = 16
	for _, h := range allHashes {
		for _, n := range []int{0, 1, window - 1} {
			w := rollinghash.NewBatchWriter(h.new(), window)
			if _, err := w.Write(make([]byte, n)); err != nil {
				t.Fatal(err)
			}
			if w.Next() {
				t.Errorf("[%s] n=%d: expected no batch before Close", h.name, n)
			}
			w.Close()
			if w.Next() {
				t.Errorf("[%s] n=%d: expected no batch after Close", h.name, n)
			}
			if w.Err() != nil {
				t.Errorf("[%s] n=%d: unexpected Err %v", h.name, n, w.Err())
			}
			if w.Sums() != nil || w.Bytes() != nil {
				t.Errorf("[%s] n=%d: expected nil Sums/Bytes", h.name, n)
			}
		}

		w := rollinghash.NewBatchWriter(h.new(), window)
		if _, err := w.Write(make([]byte, window)); err != nil {
			t.Fatal(err)
		}
		w.Close()
		got := collectBatchWriterSums(t, h.name, w, window)
		if len(got) != 1 {
			t.Errorf("[%s] exactly window: got %d sums, want 1", h.name, len(got))
		}
	}
}

// TestBatchWriterPrePostClose verifies the documented Next() semantics: it
// returns false without emitting anything before Close when there isn't a
// full window's worth of new bytes yet, and after Close it drains the
// remaining state before settling to false for good.
func TestBatchWriterPrePostClose(t *testing.T) {
	const window = 16
	w := rollinghash.NewBatchWriter(allHashes[0].new(), window)

	if _, err := w.Write(make([]byte, window-1)); err != nil {
		t.Fatal(err)
	}
	if w.Next() {
		t.Fatal("expected Next() to return false pre-Close with fewer than window bytes buffered")
	}

	if _, err := w.Write(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	if !w.Next() {
		t.Fatal("expected Next() to return true once a full window is available")
	}
	if w.Next() {
		t.Fatal("expected Next() to return false again pre-Close once drained")
	}

	w.Close()
	if w.Next() {
		t.Fatal("expected Next() to return false post-Close with nothing left")
	}
	if err := w.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
}

// TestBatchWriterError verifies that Write never errors (matching the
// documented contract) even after Close.
func TestBatchWriterError(t *testing.T) {
	w := rollinghash.NewBatchWriter(allHashes[0].new(), 16)
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestBatchWriterReset verifies that Reset lets one BatchWriter process
// multiple streams, matching a fresh BatchWriter for each.
func TestBatchWriterReset(t *testing.T) {
	const window = 16
	data := testData(5000)
	want := batchRollOracle(allHashes[0].new(), data, window)

	w := rollinghash.NewBatchWriter(allHashes[0].new(), window)
	for range 3 {
		w.Reset()
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
		w.Close()
		got := collectBatchWriterSums(t, "reset", w, window)
		equalSums(t, "reset", got, want)
	}
}

// FuzzBatchWriter cross-checks BatchWriter against the same oracle used for
// BatchRoller, feeding the input in randomly sized Write calls.
func FuzzBatchWriter(f *testing.F) {
	f.Add([]byte("The quick brown fox jumps over the lazy dog"), 4, 16, 3)
	f.Add(testData(9000), 16, 64, 17)
	f.Add([]byte("hello"), 1, 1, 1)

	f.Fuzz(func(t *testing.T, data []byte, window int, bufSize int, writeSize int) {
		if len(data) == 0 || window < 1 || window > len(data) {
			return
		}
		if bufSize < window {
			bufSize = window
		}
		if bufSize > window+(1<<16) {
			bufSize = window + (1 << 16)
		}
		if writeSize < 1 {
			writeSize = 1
		}

		for _, hc := range allHashes {
			want := batchRollOracle(hc.new(), data, window)
			w := rollinghash.NewBatchWriter(hc.new(), window, rollinghash.WithBufferSize(bufSize))
			got := writeInChunks(t, w, data, window, writeSize)
			equalSums(t, hc.name, got, want)
		}
	})
}
