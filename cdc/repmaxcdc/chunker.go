// Package repmaxcdc splits a stream into content-defined chunks using RepMaxCDC
// ("repeated maximum"), the tight-bound chunker from buildbarn/go-cdc that is
// one of the standard CDC functions proposed for Bazel's remote-execution
// protocol.
//
// It builds on MaxCDC: over a windowless accumulating Gear fingerprint
// (h = (h<<1) + gear[b], a 64-byte effective window) it looks for the position
// in a read-ahead horizon where the fingerprint is maximal, and cuts there.
// RepMaxCDC then repeats that search whenever the chosen maximum would make the
// chunk too large, restricting the horizon to just before it, until every chunk
// lands in [minSize, 2*minSize). The strict size bound makes it trivial to tell
// from its length alone whether a blob is already chunked.
//
// Parameters are minSize and a horizon size (unlike the min/normal/max of
// fastcdc and ultracdc): the horizon only controls chunking quality — a cut is
// always at least as good as the best position within
// [minSize, minSize+horizon] — and can be raised freely, with diminishing
// returns. The maximum chunk size is fixed at 2*minSize (exclusive).
//
// Boundaries match buildbarn/go-cdc's NewRepMaxContentDefinedChunker /
// NewSimpleRepMaxContentDefinedChunker for the same Gear table, minSize and
// horizon. The Gear table is supplied by the caller through the hash passed to
// New (any hash exposing Table(); gearhash64 does), so keyed chunking is just a
// keyed table. buildbarn's own default is the well-known FastCDC gear table
// (nlfiedler/fastcdc-rs, buildbuddy-io/fastcdc2020); pass that same table to
// reproduce its output.
//
// New returns a pull-based *Chunker (rollinghash.Chunker) over an io.Reader;
// NewChunkWriter returns the push-based *ChunkWriter (rollinghash.ChunkWriter),
// fed via Write/Close.
package repmaxcdc

import (
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/chunkcore"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/gearscan"
)

// window is the number of trailing bytes the Gear fingerprint depends on. The
// fingerprint is windowless but its 64-bit accumulator retains only the last 64
// bytes, and RepMaxCDC seeds each search from exactly this many bytes.
const window = 64

type gearTabler interface {
	Table() [256]uint64
}

// A Chunker splits an io.Reader into content-defined chunks using RepMaxCDC.
//
//	c := repmaxcdc.New(r, gearhash64.New(), minSize, horizon)
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
type Option func(*rmCut)

// WithBuffer supplies the working buffer, adopted when its capacity is large
// enough (roughly 2*(2*minSize+horizon)). Use it to reuse one allocation across
// many streams.
func WithBuffer(buf []byte) Option {
	return func(f *rmCut) { f.buf = buf }
}

// New returns a Chunker over r. Chunk lengths are kept in [minSize, 2*minSize);
// horizon is the read-ahead within which the repeated-maximum search always
// finds the optimal cut (0 gives uniform minSize chunks). h must expose
// Table(); New panics otherwise, or if minSize < 64 or horizon < 0.
func New(r io.Reader, h rollinghash.Hash, minSize, horizon int, opts ...Option) *Chunker {
	f := newCut(h, minSize, horizon, opts)
	return &Chunker{core: chunkcore.New(r, f, f.buf)}
}

func newCut(h rollinghash.Hash, minSize, horizon int, opts []Option) *rmCut {
	ht, ok := h.(gearTabler)
	if !ok {
		panic("repmaxcdc: requires a Gear hash exposing Table()")
	}
	if minSize < window {
		panic("repmaxcdc: minSize must be >= 64 (the Gear window)")
	}
	if horizon < 0 {
		panic("repmaxcdc: horizon must be >= 0")
	}
	f := &rmCut{
		g:    ht.Table(),
		min:  minSize,
		peek: 2*minSize + horizon,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// rmCut is the RepMaxCDC cutpoint finder: a port of buildbarn/go-cdc's
// simpleRepMaxChunkReader.ReadNextChunk.
type rmCut struct {
	g    [256]uint64
	min  int
	peek int // 2*min + horizon, the read-ahead buildbarn peeks per chunk
	buf  []byte
}

func (f *rmCut) MaxSize() int { return f.peek }

// Window: the Gear fingerprint is windowless but its accumulator retains only
// the last 64 bytes.
func (f *rmCut) Window() int { return window }

// WindowDigest returns the Gear fingerprint of b, used for Sum at a forced or
// final cut.
func (f *rmCut) WindowDigest(b []byte) uint64 { return gearscan.Digest(&f.g, b) }

func (f *rmCut) Cut(d []byte, eof bool) (int, bool, uint64) {
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

	// Repeated maximum: take the highest-fingerprint position in the horizon;
	// if cutting there would exceed 2*min, restrict the horizon to just before
	// it and search again. The horizon shrinks every iteration, so this
	// converges (worst case to a cut at exactly min).
	for {
		off, fp := gearscan.Max(g, d, min, len(d), seed)
		if off < min {
			return min + off, true, fp
		}
		d = d[:off]
	}
}

// Reset prepares the Chunker to split r from the start, reusing its buffers.
func (c *Chunker) Reset(r io.Reader) { c.core.Reset(r) }

// Next advances to the next chunk, returning false at end of input or on the
// first error.
func (c *Chunker) Next() bool { return c.core.Next() }

// Bytes returns the current chunk, valid until the next call to Next.
func (c *Chunker) Bytes() []byte { return c.core.Bytes() }

// ContentDefined reports whether the current chunk ended at a content-defined
// boundary (the repeated-maximum search picked it) rather than being forced at
// the end of the stream.
func (c *Chunker) ContentDefined() bool { return c.core.ContentDefined() }

// Sum returns the Gear fingerprint of the 64-byte window ending at the current
// chunk's cut. At a content-defined boundary it is the maximal value the
// repeated-maximum search selected. It is 0 only for a final chunk within 64
// bytes of the start of the stream.
func (c *Chunker) Sum() uint64 { return c.core.Sum() }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *Chunker) Offset() int { return c.core.Offset() }

// WindowSize returns 64: the Gear fingerprint is windowless, but its
// accumulator retains only the last 64 bytes.
func (c *Chunker) WindowSize() int { return c.core.WindowSize() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.core.Err() }
