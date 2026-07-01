package rollinghash

// batchWriter is the push-based counterpart to batchRoller: instead of
// pulling from an io.Reader, it's fed via Write. It shares batchRollerCore
// with batchRoller (see batchroller.go) — Write/Close only decide when
// core.feed/core.finish are called; the batching logic itself lives
// entirely in the core.
type batchWriter struct {
	core *batchRollerCore
}

// NewBatchWriter returns a BatchWriter. window must be >= 1. h must
// implement BatchRoll; NewBatchWriter panics otherwise. Use WithBufferSize
// to control the batch size (default 64 KiB).
func NewBatchWriter(h Hash, window int, opts ...batchRollerOption) BatchWriter {
	core := newBatchRollerCore(h, window, defaultBatchRollerBufSize)
	for _, opt := range opts {
		opt(core)
	}
	return &batchWriter{core: core}
}

// Write feeds p into the roller. It never errors and always consumes all of
// p; call Next in a loop afterward to drain any batches it completed.
func (w *batchWriter) Write(p []byte) (int, error) {
	w.core.feed(p)
	return len(p), nil
}

// Close signals that no more data will be written, so Next can emit the
// final, possibly short, batch. It always returns nil.
func (w *batchWriter) Close() error {
	w.core.finish()
	return nil
}

// Next advances to the next batch, returning false when none is available
// yet (before Close) or when everything has been emitted (after Close).
func (w *batchWriter) Next() bool { return w.core.next() == emitted }

// Sums returns the checksums of the current batch, one per window position.
// It is valid only until the next call to Next. Before the first call to
// Next, and after Next returns false, Sums returns nil.
func (w *batchWriter) Sums() []uint64 { return w.core.Sums() }

// Bytes returns the bytes of the current batch. Sums()[i] is the checksum of
// Bytes()[i:i+window]. It is valid only until the next call to Next. Before
// the first call to Next, and after Next returns false, Bytes returns nil.
func (w *batchWriter) Bytes() []byte { return w.core.Bytes() }

// Err returns the first error encountered, if any.
func (w *batchWriter) Err() error { return w.core.Err() }

// Offset returns the stream position of Bytes()[0] in the current batch.
func (w *batchWriter) Offset() int { return w.core.Offset() }

// WindowSize returns the rolling window size passed to NewBatchWriter.
func (w *batchWriter) WindowSize() int { return w.core.WindowSize() }

// Reset clears all buffered state for reuse with a new stream, keeping
// internal allocations.
func (w *batchWriter) Reset() { w.core.reset() }
