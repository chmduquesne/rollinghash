// Package repmaxsfxcdc splits a stream into content-defined chunks using
// RepMaxSfxCDC ("repeated maximum, suffix"), from buildbarn/go-cdc.
//
// Like RepMaxCDC it produces chunks strictly in [minSize, 2*minSize) by
// repeatedly picking an extremum within a read-ahead horizon and restricting
// the horizon when the pick would overshoot. The difference is what "extremum"
// means: RepMaxSfxCDC uses no rolling hash. It cuts before the position whose
// following minSize bytes form the lexicographically largest string, comparing
// those minSize-byte strings directly. Not depending on a fixed hash window
// makes cut-point selection more stable.
//
// Parameters are minSize and a horizon size (as in repmaxcdc): the horizon only
// controls chunking quality and can be raised freely. The maximum chunk size is
// fixed at 2*minSize (exclusive); the trailing chunk of a stream may be
// shorter.
//
// Boundaries match buildbarn/go-cdc's NewRepMaxSfxContentDefinedChunker (with
// the identity substitution box) and NewSimpleRepMaxSfxContentDefinedChunker for
// the same minSize and horizon.
//
// Performance note: this is a port of buildbarn's *simple* reference, with a
// first-byte prefilter so most candidate positions cost a single byte compare.
// The whole horizon is still rescanned per chunk, and on long runs of
// near-identical bytes the full minSize-byte comparisons return, degrading
// toward O(minSize · horizon). buildbarn's non-simple implementation handles
// the periodic case in linear time (and applies a substitution box to break up
// sorted input); porting that is future work.
//
// New returns a pull-based *Chunker (rollinghash.Chunker) over an io.Reader;
// NewChunkWriter returns the push-based *ChunkWriter (rollinghash.ChunkWriter),
// fed via Write/Close.
package repmaxsfxcdc

import (
	"bytes"
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/chunkcore"
)

// A Chunker splits an io.Reader into content-defined chunks using RepMaxSfxCDC.
//
//	c := repmaxsfxcdc.New(r, minSize, horizon)
//	for c.Next() {
//		chunk := c.Bytes()
//		if c.ContentDefined() { /* content-defined boundary */ }
//	}
//	if err := c.Err(); err != nil { ... }
type Chunker struct {
	core *chunkcore.Core
}

var _ rollinghash.Chunker = (*Chunker)(nil)

// Option configures New.
type Option func(*sfxCut)

// WithBuffer supplies the working buffer, adopted when its capacity is large
// enough (roughly 2*(2*minSize+horizon)). Use it to reuse one allocation across
// many streams.
func WithBuffer(buf []byte) Option {
	return func(f *sfxCut) { f.buf = buf }
}

// New returns a Chunker over r. Chunk lengths are kept in [minSize, 2*minSize);
// horizon is the read-ahead within which the repeated-maximum search always
// finds the optimal cut (0 gives uniform minSize chunks). New panics if
// minSize < 2 or horizon < 0.
func New(r io.Reader, minSize, horizon int, opts ...Option) *Chunker {
	f := newCut(minSize, horizon, opts)
	return &Chunker{core: chunkcore.New(r, f, f.buf)}
}

func newCut(minSize, horizon int, opts []Option) *sfxCut {
	if minSize < 2 {
		panic("repmaxsfxcdc: minSize must be >= 2")
	}
	if horizon < 0 {
		panic("repmaxsfxcdc: horizon must be >= 0")
	}
	f := &sfxCut{min: minSize, peek: 2*minSize + horizon}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// sfxCut is the RepMaxSfxCDC cutpoint finder: a port of buildbarn/go-cdc's
// simpleRepMaxSfxChunkReader.ReadNextChunk.
type sfxCut struct {
	min  int
	peek int // 2*min + horizon, the read-ahead buildbarn peeks per chunk
	buf  []byte
}

func (f *sfxCut) MaxSize() int { return f.peek }

// Window: RepMaxSfxCDC has no rolling window; the strings it compares start at
// the candidate cut and reach forward into the next chunk. One byte is reported
// so Sum at a forced or final cut is well defined.
func (f *sfxCut) Window() int { return 1 }

// WindowDigest returns the value of the last byte of b, the Sum at a forced or
// final cut.
func (f *sfxCut) WindowDigest(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	return uint64(b[len(b)-1])
}

func (f *sfxCut) Cut(d []byte, eof bool) (int, bool, uint64) {
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

	// Repeated maximum: the cut is the position in [min, len(d)-min] whose
	// following min bytes are lexicographically largest (ties resolve to the
	// earliest position). If that would make the chunk >= 2*min, restrict the
	// horizon to just before it and search again. The horizon shrinks every
	// iteration, so this converges (worst case to a cut at exactly min).
	for {
		best := min
		bestGram := d[min : 2*min]
		bestFirst := bestGram[0]
		for i := min + 1; i <= len(d)-min; i++ {
			// bytes.Compare(bestGram, d[i:i+min]) < 0 decomposed: a first
			// byte that loses can be rejected without touching the rest,
			// and one that strictly wins needs no comparison at all. On
			// non-degenerate data this skips almost every full compare.
			if c := d[i]; c > bestFirst || (c == bestFirst && bytes.Compare(bestGram, d[i:i+min]) < 0) {
				best, bestGram, bestFirst = i, d[i:i+min], c
			}
		}
		if best < 2*min {
			return best, true, uint64(bestFirst)
		}
		d = d[:best]
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

// Sum returns the first byte of the lexicographically-maximal minSize-byte
// string the cut was chosen for (i.e. the first byte of the next chunk). At a
// forced cut it is the final byte of the chunk instead. RepMaxSfxCDC uses no
// rolling hash, and this single byte carries less information than the
// Gear-based chunkers' Sum.
func (c *Chunker) Sum() uint64 { return c.core.Sum() }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *Chunker) Offset() int { return c.core.Offset() }

// WindowSize returns 1: RepMaxSfxCDC has no rolling window (its comparison
// strings reach forward from the cut); one byte is reported for a well-defined
// forced-cut Sum.
func (c *Chunker) WindowSize() int { return c.core.WindowSize() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.core.Err() }
