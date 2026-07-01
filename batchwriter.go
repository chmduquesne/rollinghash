package rollinghash

// batchWriter is the push-based counterpart to batchRoller: instead of
// pulling from an io.Reader, it's fed via Write. It shares batchRollerCore
// with batchRoller (see batchroller.go); Write/Close only decide when
// core.feed/core.finish are called; the batching logic itself lives
// entirely in the core.
//
// Write coalesces into pending rather than calling core.feed on every call,
// for the same reason as chunkWriter: BatchRoll needs many window positions
// per call to pay off. pending is flushed once it reaches core.batchSize
// bytes (see WithBufferSize), or on Close. This trades Write-to-Next
// latency for throughput on fragmented input. The tradeoff is steep at the
// low end: WithBufferSize(window), the technical minimum, gives up most of
// the throughput a batch size of just a few KiB would already recover.
// Prefer a small-but-not-minimal value (low single-digit KiB) over window
// if lower latency is needed; only drop all the way to window if the
// workload truly can't tolerate a KiB-scale buffering delay.
type batchWriter struct {
	core    *batchRollerCore
	pending []byte
}

// NewBatchWriter returns a BatchWriter. window must be >= 1. h must
// implement BatchRoll; NewBatchWriter panics otherwise. Use WithBufferSize
// to control the batch size (default 64 KiB); this doubles as the
// write-coalescing threshold; see the chunkWriter/batchWriter doc comment.
func NewBatchWriter(h Hash, window int, opts ...batchRollerOption) BatchWriter {
	core := newBatchRollerCore(h, window, defaultBatchRollerBufSize)
	for _, opt := range opts {
		opt(core)
	}
	return &batchWriter{core: core}
}

// Write feeds p into the roller. It always consumes all of p; call Next in
// a loop afterward to drain any batches it completed. Write returns
// ErrClosed if called after Close.
func (w *batchWriter) Write(p []byte) (int, error) {
	if w.core.eof {
		return 0, ErrClosed
	}
	n := len(p)
	batchSize := max(w.core.batchSize, 1)

	// Top up any leftover from a previous Write with just enough of p's
	// head to complete one batch, rather than absorbing all of p into
	// pending. This keeps pending bounded to at most batchSize-1 bytes
	// (and its backing array with it) no matter how large individual
	// Write calls are; only ever copying a whole large Write into pending
	// would otherwise repin its backing array to that write's size.
	if len(w.pending) > 0 {
		need := batchSize - len(w.pending)
		if need > len(p) {
			w.pending = append(w.pending, p...)
			return n, nil
		}
		w.pending = append(w.pending, p[:need]...)
		w.core.feed(w.pending)
		w.pending = w.pending[:0]
		p = p[need:]
	}

	// Feed directly from p in batchSize slices: zero-copy regardless of
	// whether this is the first Write of the stream or a later one.
	for len(p) >= batchSize {
		w.core.feed(p[:batchSize])
		p = p[batchSize:]
	}
	w.pending = append(w.pending[:0], p...)
	return n, nil
}

// Close signals that no more data will be written, so Next can emit the
// final, possibly short, batch. It flushes any bytes still held back by
// Write's coalescing. It always returns nil; further Writes return
// ErrClosed.
func (w *batchWriter) Close() error {
	if len(w.pending) > 0 {
		w.core.feed(w.pending)
		w.pending = w.pending[:0]
	}
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
// internal allocations. It also un-closes the writer: Write may be called
// again after Reset even if Close was called before it.
func (w *batchWriter) Reset() {
	w.core.reset()
	w.pending = w.pending[:0]
}
