// Package ramcdc splits a stream into content-defined chunks using RAM (Rapid
// Asymmetric Maximum), from the paper "RapidCDC: Leveraging Duplicate Locality
// to Accelerate CDC" lineage — specifically the RAM algorithm of Widodo et al.,
// "A New Content-Defined Chunking Algorithm for Data Deduplication in Cloud
// Storage" (2017), in the variant benchmarked by UWASL/dedup-bench.
//
// RAM uses no rolling hash. At the start of each chunk it takes a fixed
// windowSize-byte window and finds its maximum byte; it then scans forward from
// the end of that window and cuts before the first byte that is greater than or
// equal to that maximum — an asymmetric pairing of a fixed left window with an
// unbounded right scan. A hard maxSize forces a cut when no such byte appears.
// The trailing chunk of a stream may be shorter than windowSize.
//
// This is one of the three hashless algorithms VectorCDC accelerates: the window
// maximum is an Extreme Byte Search and the forward scan is a Range Scan, both
// run through cdc/internal/vectorscan (AVX2 on amd64, a portable byte loop
// elsewhere). Boundaries are identical to the scalar algorithm either way.
//
// New returns a pull-based *Chunker (rollinghash.Chunker) over an io.Reader;
// NewChunkWriter returns the push-based *ChunkWriter (rollinghash.ChunkWriter),
// fed via Write/Close.
package ramcdc

import (
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/cutcore"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/vectorscan"
)

// A Chunker splits an io.Reader into content-defined chunks using RAM.
//
//	c := ramcdc.New(r, windowSize, maxSize)
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
type Option func(*ramCut)

// WithBuffer supplies the working buffer, adopted when its capacity is large
// enough (roughly 2*maxSize). Use it to reuse one allocation across many
// streams.
func WithBuffer(buf []byte) Option {
	return func(f *ramCut) { f.buf = buf }
}

// New returns a Chunker over r. windowSize is the fixed left window RAM takes at
// each chunk start; content-defined chunks are in [windowSize, maxSize] and a
// final chunk may be shorter. New panics if windowSize < 1 or maxSize < windowSize.
func New(r io.Reader, windowSize, maxSize int, opts ...Option) *Chunker {
	f := newCut(windowSize, maxSize, opts)
	return &Chunker{core: cutcore.New(r, f, f.buf)}
}

func newCut(windowSize, maxSize int, opts []Option) *ramCut {
	if windowSize < 1 {
		panic("ramcdc: windowSize must be >= 1")
	}
	if maxSize < windowSize {
		panic("ramcdc: maxSize must be >= windowSize")
	}
	f := &ramCut{w: windowSize, max: maxSize}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// ramCut is the RAM cutpoint finder: a port of dedup-bench's
// RAM_Chunking::find_cutpoint (native branch).
type ramCut struct {
	w   int
	max int
	buf []byte
}

func (f *ramCut) MaxSize() int { return f.max }

// Window: RAM's boundary hinges on the window maximum and one trailing byte; one
// byte is reported so Sum at a forced or final cut is well defined.
func (f *ramCut) Window() int { return 1 }

// WindowDigest returns the value of the last byte of b, the RAM Sum at a forced
// or final cut.
func (f *ramCut) WindowDigest(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	return uint64(b[len(b)-1])
}

func (f *ramCut) Cut(d []byte, eof bool) (int, bool, uint64) {
	if len(d) > f.max {
		d = d[:f.max]
	}
	n := len(d)
	w := f.w
	if n <= w {
		// Not enough bytes past the window to place a boundary: forced tail.
		return n, false, uint64(d[n-1])
	}
	mx := vectorscan.MaxByte(d[:w])
	rel := vectorscan.IndexGE(d[w:n], mx)
	if rel == n-w {
		// No byte >= the window maximum before maxSize / the stream end.
		return n, false, uint64(d[n-1])
	}
	return w + rel, true, uint64(mx)
}

// Reset prepares the Chunker to split r from the start, reusing its buffers.
func (c *Chunker) Reset(r io.Reader) { c.core.Reset(r) }

// Next advances to the next chunk, returning false at end of input or on the
// first error.
func (c *Chunker) Next() bool { return c.core.Next() }

// Bytes returns the current chunk, valid until the next call to Next.
func (c *Chunker) Bytes() []byte { return c.core.Bytes() }

// ContentDefined reports whether the current chunk ended at a content-defined
// boundary (a byte >= the window maximum was found) rather than being forced at
// maxSize or the end of the stream.
func (c *Chunker) ContentDefined() bool { return c.core.ContentDefined() }

// Sum returns the maximum byte of the chunk's leading windowSize bytes — the
// value RAM's forward scan tested against. At a forced or final cut it is the
// last byte of the chunk instead. RAM uses no rolling hash.
func (c *Chunker) Sum() uint64 { return c.core.Sum() }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *Chunker) Offset() int { return c.core.Offset() }

// WindowSize returns 1: RAM has no rolling window; one byte is reported for a
// well-defined forced-cut Sum.
func (c *Chunker) WindowSize() int { return c.core.WindowSize() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.core.Err() }
