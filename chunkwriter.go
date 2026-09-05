package rollinghash

import "math"

// chunkWriter is the push-based counterpart to chunker: instead of pulling
// from an io.Reader, it's fed via Write. It shares splitter with chunker
// (see chunker.go); Write/Close only decide when splitter.feed/splitter.finish
// are called; the boundary-finding logic itself lives entirely in the
// splitter.
//
// Write coalesces into pending rather than calling splitter.feed on every
// call: invoking BatchBoundaries on tiny, frequent writes would pay its
// per-call overhead without the ILP benefit it needs many window positions to
// exploit. pending is flushed once it reaches splitter.batchSize bytes (see
// WithBatchSize), or on Close. This trades Write-to-Next latency for
// throughput on fragmented input. The tradeoff is steep at the low end:
// WithBatchSize(window), the technical minimum, measured at roughly a
// third of the default's throughput in this package's benchmarks, while a
// batch size of just a few KiB already recovers nearly all of it. Prefer a
// small-but-not-minimal value (low single-digit KiB) over window if lower
// latency is needed; only drop all the way to window if the workload truly
// can't tolerate a KiB-scale buffering delay.
type chunkWriter struct {
	splitter *splitter
	pending  []byte
}

var _ ChunkWriter = (*chunkWriter)(nil)

// NewChunkWriter returns a ChunkWriter. A boundary is placed where the
// rolling checksum under h (over window bytes) satisfies checksum & mask ==
// 0, with the chunk length kept in [min, max] (see WithBoundaries). window
// must be >= 1. The hash must implement BatchBoundaries; NewChunkWriter
// panics otherwise.
func NewChunkWriter(h Hash, window int, mask uint64, opts ...chunkerOption) ChunkWriter {
	sp := newSplitter(h, window, mask, 0, math.MaxInt)
	for _, opt := range opts {
		opt(sp)
	}
	return &chunkWriter{splitter: sp}
}

// Write feeds p into the chunker. It always consumes all of p; call Next in
// a loop afterward to drain any chunks it completed. Write returns
// ErrClosed if called after Close.
func (w *chunkWriter) Write(p []byte) (int, error) {
	if w.splitter.eof {
		return 0, ErrClosed
	}
	n := len(p)
	batchSize := max(w.splitter.batchSize, 1)

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
		w.splitter.feed(w.pending)
		w.pending = w.pending[:0]
		p = p[need:]
	}

	// Feed directly from p in batchSize slices: zero-copy regardless of
	// whether this is the first Write of the stream or a later one.
	for len(p) >= batchSize {
		w.splitter.feed(p[:batchSize])
		p = p[batchSize:]
	}
	w.pending = append(w.pending[:0], p...)
	return n, nil
}

// Close signals that no more data will be written, so Next can flush the
// final, possibly short, chunk. It flushes any bytes still held back by
// Write's coalescing. It always returns nil; further Writes return
// ErrClosed.
func (w *chunkWriter) Close() error {
	if len(w.pending) > 0 {
		w.splitter.feed(w.pending)
		w.pending = w.pending[:0]
	}
	w.splitter.finish()
	return nil
}

// Next advances to the next chunk, returning false when none is available
// yet (before Close) or when every chunk has been emitted (after Close).
func (w *chunkWriter) Next() bool { return w.splitter.next() == emitted }

// Bytes returns the current chunk, valid until the next call to Next. Before
// the first call to Next, and after Next returns false, Bytes returns nil.
func (w *chunkWriter) Bytes() []byte { return w.splitter.Bytes() }

// Sum returns the rolling checksum of the window ending at the current chunk's
// cut, whether the cut was a mask hit, a forced cut at max, or the end of the
// stream. It is 0 only for a final chunk whose stream has fewer than window
// bytes. Before the first call to Next, and after Next returns false, Sum
// returns 0.
func (w *chunkWriter) Sum() uint64 { return w.splitter.Sum() }

// ContentDefined reports whether the current chunk was cut by the mask
// (true) rather than forced at max or at end of stream (false).
func (w *chunkWriter) ContentDefined() bool { return w.splitter.ContentDefined() }

// Err returns the first error encountered, if any.
func (w *chunkWriter) Err() error { return w.splitter.Err() }

// Offset returns the start byte offset of the current chunk in the stream.
func (w *chunkWriter) Offset() int { return w.splitter.Offset() }

// WindowSize returns the rolling window size passed to NewChunkWriter.
func (w *chunkWriter) WindowSize() int { return w.splitter.WindowSize() }

// Reset clears all buffered state for reuse with a new stream, keeping
// internal allocations. It also un-closes the writer: Write may be called
// again after Reset even if Close was called before it.
func (w *chunkWriter) Reset() {
	w.splitter.reset()
	w.pending = w.pending[:0]
}
