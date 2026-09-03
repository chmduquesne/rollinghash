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
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/chunkcore"
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
	core *chunkcore.Core
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
	return &Chunker{core: chunkcore.New(r, f, f.buf)}
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
	min := f.min // hoist: keep the hot loop off struct fields

	// maxValue is the running maximum byte since the chunk start; nextCut is
	// min bytes past its position — the cut point, reached once that many
	// non-exceeding bytes have followed. Tracking nextCut directly avoids an
	// add per byte.
	//
	// The scan runs eight bytes per iteration. The recurrence is serial, but
	// issuing the eight comparisons as a straight-line group (rather than one
	// per loop turn behind the loop-bound and cut-point branches) lets them
	// pipeline — ~30% faster here, and the Go compiler will not unroll it. The
	// scalar tail loop below is the real specification.
	maxValue := d[0]
	nextCut := 1 + min
	i := 2
	for ; i+8 <= n; i += 8 {
		s := d[i-1 : i+7 : i+7]
		if s[0] > maxValue {
			maxValue, nextCut = s[0], i+min
		} else if i == nextCut {
			return i, true, uint64(maxValue)
		}
		if s[1] > maxValue {
			maxValue, nextCut = s[1], i+1+min
		} else if i+1 == nextCut {
			return i + 1, true, uint64(maxValue)
		}
		if s[2] > maxValue {
			maxValue, nextCut = s[2], i+2+min
		} else if i+2 == nextCut {
			return i + 2, true, uint64(maxValue)
		}
		if s[3] > maxValue {
			maxValue, nextCut = s[3], i+3+min
		} else if i+3 == nextCut {
			return i + 3, true, uint64(maxValue)
		}
		if s[4] > maxValue {
			maxValue, nextCut = s[4], i+4+min
		} else if i+4 == nextCut {
			return i + 4, true, uint64(maxValue)
		}
		if s[5] > maxValue {
			maxValue, nextCut = s[5], i+5+min
		} else if i+5 == nextCut {
			return i + 5, true, uint64(maxValue)
		}
		if s[6] > maxValue {
			maxValue, nextCut = s[6], i+6+min
		} else if i+6 == nextCut {
			return i + 6, true, uint64(maxValue)
		}
		if s[7] > maxValue {
			maxValue, nextCut = s[7], i+7+min
		} else if i+7 == nextCut {
			return i + 7, true, uint64(maxValue)
		}
	}
	for ; i < n; i++ {
		if b := d[i-1]; b > maxValue {
			maxValue, nextCut = b, i+min
		} else if i == nextCut {
			return i, true, uint64(maxValue)
		}
	}
	return n, false, uint64(maxValue)
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
