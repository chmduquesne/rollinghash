// Package aecdc splits a stream into content-defined chunks using AE (Asymmetric
// Extremum), from the paper "AE: An Asymmetric Extremum Content Defined Chunking
// Algorithm for Fast and Bandwidth-Efficient Data Deduplication" (Zhang et al.,
// 2015), in the variant implemented by buildbarn/go-cdc.
//
// AE uses no rolling hash. It scans raw byte values, tracking the running
// maximum since the last cut, and cuts minSize bytes after that maximum's
// position once minSize bytes have gone by without a larger value — an
// unbounded left window paired with a fixed-size right window (hence
// "asymmetric"). Content-defined chunks are therefore at least minSize+1 bytes;
// a hard maxSize forces a cut when no extremum settles in time. The trailing
// chunk of a stream may be shorter than minSize+1.
//
// Boundaries match buildbarn/go-cdc's NewAsymmetricExtremumContentDefinedChunker
// / NewSimpleAsymmetricExtremumContentDefinedChunker for the same minSize and
// maxSize.
//
// New returns a pull-based *Chunker (rollinghash.Chunker) over an io.Reader;
// NewChunkWriter returns the push-based *ChunkWriter (rollinghash.ChunkWriter),
// fed via Write/Close.
package aecdc

import (
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/cutcore"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/vectorscan"
)

// A Chunker splits an io.Reader into content-defined chunks using AE.
//
//	c := aecdc.New(r, minSize, maxSize)
//	for c.Next() {
//		chunk := c.Bytes()
//		if c.ContentDefined() { /* content-defined boundary */ }
//	}
//	if err := c.Err(); err != nil { ... }
type Chunker struct {
	core *cutcore.Core
}

var _ rollinghash.Chunker = (*Chunker)(nil)

// Option configures New.
type Option func(*aeCut)

// WithBuffer supplies the working buffer, adopted when its capacity is large
// enough (roughly 2*maxSize). Use it to reuse one allocation across many
// streams.
func WithBuffer(buf []byte) Option {
	return func(f *aeCut) { f.buf = buf }
}

// New returns a Chunker over r. Content-defined chunks are in [minSize+1,
// maxSize]; a final chunk may be shorter. New panics if minSize < 1 or
// maxSize < minSize.
func New(r io.Reader, minSize, maxSize int, opts ...Option) *Chunker {
	f := newCut(minSize, maxSize, opts)
	return &Chunker{core: cutcore.New(r, f, f.buf)}
}

func newCut(minSize, maxSize int, opts []Option) *aeCut {
	if minSize < 1 {
		panic("aecdc: minSize must be >= 1")
	}
	if maxSize < minSize {
		panic("aecdc: maxSize must be >= minSize")
	}
	f := &aeCut{min: minSize, max: maxSize}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// aeCut is the AE cutpoint finder: a port of buildbarn/go-cdc's
// simpleAsymmetricExtremumChunkReader.ReadNextChunk.
type aeCut struct {
	min int
	max int
	buf []byte
}

func (f *aeCut) MaxSize() int { return f.max }

// Window: AE has no rolling window; the boundary test spans the whole chunk.
// One byte is reported so Sum at a forced or final cut is well defined.
func (f *aeCut) Window() int { return 1 }

// WindowDigest returns the value of the last byte of b, the AE Sum at a forced
// or final cut.
func (f *aeCut) WindowDigest(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	return uint64(b[len(b)-1])
}

func (f *aeCut) Cut(d []byte, eof bool) (int, bool, uint64) {
	if len(d) > f.max {
		d = d[:f.max]
	}
	n := len(d)
	msz := f.min

	// AE decomposed into VectorCDC's two primitives, producing the exact same
	// boundaries as the plain scan (see refAE in chunker_test.go, and the
	// buildbarn parity test): d[0] is the first target byte; a Range Scan finds
	// the next byte strictly greater than the running maximum, and the chunk is
	// cut min bytes past a target that no later byte in that window exceeds.
	//
	// The classic scan cuts at chunk length i when byte d[i-1] is the running
	// max and the min bytes after it are all <= it. maxPos tracks that target's
	// i (its byte index is maxPos-1); c = maxPos + min is the candidate cut. The
	// scan never inspects the final byte d[n-1], so the exceed search stops at
	// n-1.
	if n < 3 {
		return n, false, uint64(d[n-1])
	}
	maxValue := d[0]
	maxPos := 1
	for {
		c := maxPos + msz
		hi := min(c, n-1)
		if maxPos >= hi {
			// The scan runs out of bytes before reaching c: forced cut.
			return n, false, uint64(d[n-1])
		}
		w := d[maxPos:hi]
		if rel := vectorscan.IndexGT(w, maxValue); rel < len(w) {
			e := maxPos + rel // byte index of the new, larger target
			maxValue = d[e]
			maxPos = e + 1
			continue
		}
		if hi == c {
			return c, true, uint64(maxValue)
		}
		return n, false, uint64(d[n-1])
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
// boundary (an extremum settled) rather than being forced at maxSize or the end
// of the stream.
func (c *Chunker) ContentDefined() bool { return c.core.ContentDefined() }

// Sum returns the extremum (maximum) byte value the boundary test selected for
// the current chunk's cut. At a forced cut it is the final byte of the chunk
// instead. AE uses no rolling hash; this is the value its boundary test hinges
// on.
func (c *Chunker) Sum() uint64 { return c.core.Sum() }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *Chunker) Offset() int { return c.core.Offset() }

// WindowSize returns 1: AE has no rolling window (its left window spans the
// whole chunk); one byte is reported for a well-defined forced-cut Sum.
func (c *Chunker) WindowSize() int { return c.core.WindowSize() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.core.Err() }
