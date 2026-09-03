// Package maxcdc splits a stream into content-defined chunks using MaxCDC, the
// "cut at the local maximum" chunker from buildbarn/go-cdc.
//
// Over a windowless accumulating Gear fingerprint (h = (h<<1) + gear[b], a
// 64-byte effective window) it scans the read-ahead region [minSize, maxSize]
// from the current chunk start and cuts before the position where the
// fingerprint is largest. Unlike FastCDC there is no boundary mask: every chunk
// ends at the maximum of its horizon, so chunk lengths land in [minSize,
// maxSize] with no probabilistic tail.
//
// RepMaxCDC (cdc/repmaxcdc) refines this into a strict [minSize, 2*minSize)
// bound; MaxCDC degrades if maxSize/minSize is large, but is simpler and needs
// only one pass over each horizon.
//
// Boundaries match buildbarn/go-cdc's NewMaxContentDefinedChunker /
// NewSimpleMaxContentDefinedChunker for the same Gear table, minSize and
// maxSize. The Gear table is supplied by the caller through the hash passed to
// New (any hash exposing Table(); gearhash64 does), so keyed chunking is just a
// keyed table.
//
// New returns a pull-based *Chunker (rollinghash.Chunker) over an io.Reader;
// NewChunkWriter returns the push-based *ChunkWriter (rollinghash.ChunkWriter),
// fed via Write/Close.
package maxcdc

import (
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/chunkcore"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/gearscan"
)

// window is the number of trailing bytes the Gear fingerprint depends on. The
// fingerprint is windowless but its 64-bit accumulator retains only the last 64
// bytes, and MaxCDC seeds each search from exactly this many bytes.
const window = 64

type gearTabler interface {
	Table() [256]uint64
}

// A Chunker splits an io.Reader into content-defined chunks using MaxCDC.
//
//	c := maxcdc.New(r, gearhash64.New(), minSize, maxSize)
//	for c.Next() {
//		chunk := c.Bytes()
//		if c.ContentDefined() { /* content-defined boundary */ }
//	}
//	if err := c.Err(); err != nil { ... }
//
// The hash must expose Table() (gearhash64 does); New panics otherwise.
type Chunker struct {
	core *chunkcore.Core
}

var _ rollinghash.Chunker = (*Chunker)(nil)

// Option configures New.
type Option func(*mxCut)

// WithBuffer supplies the working buffer, adopted when its capacity is large
// enough (roughly 2*(minSize+maxSize)). Use it to reuse one allocation across
// many streams.
func WithBuffer(buf []byte) Option {
	return func(f *mxCut) { f.buf = buf }
}

// New returns a Chunker over r. Chunk lengths are kept in [minSize, maxSize]:
// every chunk is cut before the highest-fingerprint position in that range. h
// must expose Table(); New panics otherwise, or if minSize < 64 or
// maxSize <= minSize.
func New(r io.Reader, h rollinghash.Hash, minSize, maxSize int, opts ...Option) *Chunker {
	f := newCut(h, minSize, maxSize, opts)
	return &Chunker{core: chunkcore.New(r, f, f.buf)}
}

func newCut(h rollinghash.Hash, minSize, maxSize int, opts []Option) *mxCut {
	ht, ok := h.(gearTabler)
	if !ok {
		panic("maxcdc: requires a Gear hash exposing Table()")
	}
	if minSize < window {
		panic("maxcdc: minSize must be >= 64 (the Gear window)")
	}
	if maxSize <= minSize {
		panic("maxcdc: maxSize must be > minSize")
	}
	f := &mxCut{
		g:    ht.Table(),
		min:  minSize,
		peek: minSize + maxSize,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// mxCut is the MaxCDC cutpoint finder: a port of buildbarn/go-cdc's
// simpleMaxChunkReader.ReadNextChunk.
type mxCut struct {
	g    [256]uint64
	min  int
	peek int // min + max, the read-ahead buildbarn peeks per chunk
	buf  []byte
}

func (f *mxCut) MaxSize() int { return f.peek }

// Window: the Gear fingerprint is windowless but its accumulator retains only
// the last 64 bytes.
func (f *mxCut) Window() int { return window }

// WindowDigest returns the Gear fingerprint of b, used for Sum at a forced or
// final cut.
func (f *mxCut) WindowDigest(b []byte) uint64 { return gearscan.Digest(&f.g, b) }

func (f *mxCut) Cut(d []byte, eof bool) (int, bool, uint64) {
	min := f.min

	// Cap the view to the peek horizon, matching buildbarn's
	// Peek(peekSizeBytes). The core may hand us more than MaxSize.
	if len(d) > f.peek {
		d = d[:f.peek]
	}

	// Too little left to guarantee a >= min follow-up chunk: emit everything
	// as one forced chunk. The core only calls Cut with fewer than MaxSize
	// (>= 2*min) bytes at end of stream, so this is the EOF tail.
	if len(d) < 2*min {
		return len(d), false, 0
	}

	// Reserve the trailing min bytes for the next chunk.
	d = d[:len(d)-min]

	// Seed the fingerprint over the 64 bytes ending at the earliest legal cut.
	g := &f.g
	var seed uint64
	for _, b := range d[min-window : min] {
		seed = (seed << 1) + g[b]
	}

	// Cut before the highest-fingerprint position in [min, len(d)].
	off, fp := gearscan.Max(g, d, min, len(d), seed)
	return min + off, true, fp
}

// Reset prepares the Chunker to split r from the start, reusing its buffers.
func (c *Chunker) Reset(r io.Reader) { c.core.Reset(r) }

// Next advances to the next chunk, returning false at end of input or on the
// first error.
func (c *Chunker) Next() bool { return c.core.Next() }

// Bytes returns the current chunk, valid until the next call to Next.
func (c *Chunker) Bytes() []byte { return c.core.Bytes() }

// ContentDefined reports whether the current chunk ended at a content-defined
// boundary (the local-maximum search picked it) rather than being forced at the
// end of the stream.
func (c *Chunker) ContentDefined() bool { return c.core.ContentDefined() }

// Sum returns the Gear fingerprint of the 64-byte window ending at the current
// chunk's cut. At a content-defined boundary it is the maximal value the search
// selected. It is 0 only for a final chunk within 64 bytes of the start of the
// stream.
func (c *Chunker) Sum() uint64 { return c.core.Sum() }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *Chunker) Offset() int { return c.core.Offset() }

// WindowSize returns 64: the Gear fingerprint is windowless, but its
// accumulator retains only the last 64 bytes.
func (c *Chunker) WindowSize() int { return c.core.WindowSize() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.core.Err() }
