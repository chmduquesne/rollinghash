package rollinghash

import "math"

// chunkWriter is the push-based counterpart to chunker: instead of pulling
// from an io.Reader, it's fed via Write. It shares chunkerCore with chunker
// (see chunker.go) — Write/Close only decide when core.feed/core.finish are
// called; the boundary-finding logic itself lives entirely in the core.
type chunkWriter struct {
	core *chunkerCore
}

// NewChunkWriter returns a ChunkWriter. A boundary is placed where the
// rolling checksum under h (over window bytes) satisfies checksum & mask ==
// 0, with the chunk length kept in [min, max] (see WithBoundaries). window
// must be >= 1. The hash must implement BatchBoundaries; NewChunkWriter
// panics otherwise.
func NewChunkWriter(h Hash, window int, mask uint64, opts ...chunkerOption) ChunkWriter {
	core := newChunkerCore(h, window, mask, 0, math.MaxInt)
	for _, opt := range opts {
		opt(core)
	}
	return &chunkWriter{core: core}
}

// Write feeds p into the chunker. It never errors and always consumes all
// of p; call Next in a loop afterward to drain any chunks it completed.
func (w *chunkWriter) Write(p []byte) (int, error) {
	w.core.feed(p)
	return len(p), nil
}

// Close signals that no more data will be written, so Next can flush the
// final, possibly short, chunk. It always returns nil.
func (w *chunkWriter) Close() error {
	w.core.finish()
	return nil
}

// Next advances to the next chunk, returning false when none is available
// yet (before Close) or when every chunk has been emitted (after Close).
func (w *chunkWriter) Next() bool { return w.core.next() == emitted }

// Bytes returns the current chunk, valid until the next call to Next. Before
// the first call to Next, and after Next returns false, Bytes returns nil.
func (w *chunkWriter) Bytes() []byte { return w.core.Bytes() }

// Sum returns the rolling checksum at the current chunk's boundary. Before
// the first call to Next, and after Next returns false, Sum returns 0.
func (w *chunkWriter) Sum() uint64 { return w.core.Sum() }

// ContentDefined reports whether the current chunk was cut by the mask
// (true) rather than forced at max or at end of stream (false).
func (w *chunkWriter) ContentDefined() bool { return w.core.ContentDefined() }

// Err returns the first error encountered, if any.
func (w *chunkWriter) Err() error { return w.core.Err() }

// Offset returns the start byte offset of the current chunk in the stream.
func (w *chunkWriter) Offset() int { return w.core.Offset() }

// WindowSize returns the rolling window size passed to NewChunkWriter.
func (w *chunkWriter) WindowSize() int { return w.core.WindowSize() }

// Reset clears all buffered state for reuse with a new stream, keeping
// internal allocations.
func (w *chunkWriter) Reset() { w.core.reset() }
