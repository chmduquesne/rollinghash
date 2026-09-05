package ultracdc

import (
	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/cutcore"
)

// A ChunkWriter is the push-based counterpart to Chunker: fed via Write/Close
// rather than owning an io.Reader, for callers whose data arrives in
// caller-controlled pieces (network reads, a callback API). Before Close, Next
// returning false means not enough has been written yet for a boundary; write
// more and try again. After Close, Next returning false means every chunk has
// been emitted.
type ChunkWriter struct {
	core *cutcore.Core
}

var _ rollinghash.ChunkWriter = (*ChunkWriter)(nil)

// NewChunkWriter returns the push-based counterpart to New: instead of pulling
// from an io.Reader it is fed via Write, and Close marks end of input. The
// parameters are identical to New. It implements rollinghash.ChunkWriter.
func NewChunkWriter(minSize, normalSize, maxSize int, opts ...Option) *ChunkWriter {
	f := newCut(minSize, normalSize, maxSize, opts)
	return &ChunkWriter{core: cutcore.NewWriter(f, f.buf)}
}

// Write feeds p into the chunker; it always consumes all of p and returns
// rollinghash.ErrClosed if called after Close.
func (w *ChunkWriter) Write(p []byte) (int, error) { return w.core.Write(p) }

// Close marks the end of input.
func (w *ChunkWriter) Close() error { return w.core.Close() }

// Reset clears all buffered state for reuse, keeping internal allocations.
func (w *ChunkWriter) Reset() { w.core.ResetWriter() }

// Next, Bytes, ContentDefined, Sum, Offset, WindowSize and Err behave as on
// Chunker.
func (w *ChunkWriter) Next() bool           { return w.core.Next() }
func (w *ChunkWriter) Bytes() []byte        { return w.core.Bytes() }
func (w *ChunkWriter) ContentDefined() bool { return w.core.ContentDefined() }
func (w *ChunkWriter) Sum() uint64          { return w.core.Sum() }
func (w *ChunkWriter) Offset() int          { return w.core.Offset() }
func (w *ChunkWriter) WindowSize() int      { return w.core.WindowSize() }
func (w *ChunkWriter) Err() error           { return w.core.Err() }
