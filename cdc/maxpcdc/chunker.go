// Package maxpcdc splits a stream into content-defined chunks using MAXP, also
// known as Local Maximum Chunking (LMC), from Bjørner et al., "Content-Defined
// Chunking for Files with Identical Content" / the MAXP variant benchmarked by
// UWASL/dedup-bench.
//
// Unlike cdc/maxcdc — which cuts at the maximum of a windowless Gear
// fingerprint, a buildbarn port — maxpcdc compares raw byte values. It slides a
// target byte between two windowSize-byte windows and cuts right after a target
// that is greater than or equal to every byte in the window before it and
// strictly greater than none after it: a local maximum over 2*windowSize+1
// bytes. Because MAXP's windows are small relative to AE and RAM for the same
// average size, it settles quickly. A hard maxSize forces a cut when no local
// maximum appears; the trailing chunk may be shorter than 2*windowSize+1.
//
// This is one of the three hashless algorithms VectorCDC accelerates: the
// forward search for a candidate is a Range Scan and the backward validation is
// an Extreme Byte Search, both run through cdc/internal/vectorscan (AVX2 on
// amd64, a portable byte loop elsewhere). Boundaries are identical either way.
//
// New returns a pull-based *Chunker (rollinghash.Chunker) over an io.Reader;
// NewChunkWriter returns the push-based *ChunkWriter (rollinghash.ChunkWriter),
// fed via Write/Close.
package maxpcdc

import (
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/chunkcore"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/vectorscan"
)

// A Chunker splits an io.Reader into content-defined chunks using MAXP.
//
//	c := maxpcdc.New(r, windowSize, maxSize)
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
type Option func(*maxpCut)

// WithBuffer supplies the working buffer, adopted when its capacity is large
// enough (roughly 2*maxSize). Use it to reuse one allocation across many
// streams.
func WithBuffer(buf []byte) Option {
	return func(f *maxpCut) { f.buf = buf }
}

// New returns a Chunker over r. windowSize is the size of each of MAXP's two
// windows; content-defined chunks are in [windowSize, maxSize] and a final chunk
// may be shorter. New panics if windowSize < 1 or maxSize < 2*windowSize+1.
func New(r io.Reader, windowSize, maxSize int, opts ...Option) *Chunker {
	f := newCut(windowSize, maxSize, opts)
	return &Chunker{core: chunkcore.New(r, f, f.buf)}
}

func newCut(windowSize, maxSize int, opts []Option) *maxpCut {
	if windowSize < 1 {
		panic("maxpcdc: windowSize must be >= 1")
	}
	if maxSize < 2*windowSize+1 {
		panic("maxpcdc: maxSize must be >= 2*windowSize+1")
	}
	f := &maxpCut{w: windowSize, max: maxSize}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// maxpCut is the MAXP cutpoint finder: a port of dedup-bench's
// MAXP_Chunking::find_cutpoint_native.
type maxpCut struct {
	w   int
	max int
	buf []byte
}

func (f *maxpCut) MaxSize() int { return f.max }

// Window: MAXP's boundary test spans a local region; one byte is reported so Sum
// at a forced or final cut is well defined.
func (f *maxpCut) Window() int { return 1 }

// WindowDigest returns the value of the last byte of b, the MAXP Sum at a forced
// or final cut.
func (f *maxpCut) WindowDigest(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	return uint64(b[len(b)-1])
}

func (f *maxpCut) Cut(d []byte, eof bool) (int, bool, uint64) {
	w := f.w
	if len(d) < 2*w+1 {
		return len(d), false, uint64(d[len(d)-1])
	}
	if len(d) > f.max {
		d = d[:f.max]
	}
	n := len(d)

	// maxPos/maxVal track the running maximum (updated on >=, so it advances
	// through ties). A candidate settles once w bytes pass maxPos with nothing
	// >= it; it is a boundary if no byte in the w bytes before maxPos strictly
	// exceeds it. The forward loop only ever reaches index n-2.
	maxPos := w
	maxVal := d[maxPos]
	for {
		lo := maxPos + 1
		hi := min(maxPos+w+1, n-1)
		if lo < hi {
			if rel := vectorscan.IndexGE(d[lo:hi], maxVal); rel < hi-lo {
				i := lo + rel
				maxPos, maxVal = i, d[i]
				continue
			}
		}
		cand := maxPos + w
		if cand > n-2 {
			return n, false, uint64(d[n-1])
		}
		// i == cand reached with d[cand] < maxVal: validate against the left window.
		if vectorscan.MaxByte(d[maxPos-w:maxPos]) <= maxVal {
			return maxPos, true, uint64(maxVal)
		}
		maxPos = cand + 1
		maxVal = d[maxPos]
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
// boundary (a local maximum settled) rather than being forced at maxSize or the
// end of the stream.
func (c *Chunker) ContentDefined() bool { return c.core.ContentDefined() }

// Sum returns the local-maximum byte value the boundary test selected. At a
// forced or final cut it is the last byte of the chunk instead. MAXP uses no
// rolling hash.
func (c *Chunker) Sum() uint64 { return c.core.Sum() }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *Chunker) Offset() int { return c.core.Offset() }

// WindowSize returns 1: MAXP has no rolling window; one byte is reported for a
// well-defined forced-cut Sum.
func (c *Chunker) WindowSize() int { return c.core.WindowSize() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.core.Err() }
