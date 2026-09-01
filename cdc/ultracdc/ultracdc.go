// Package ultracdc splits a stream into content-defined chunks using the
// UltraCDC algorithm. Unlike gear-based chunkers it uses no rolling hash: it
// slides an 8-byte window, tracks its Hamming distance to the constant pattern
// 0xAA..AA, and cuts when that distance clears a small mask; it also cuts on a
// long run of identical windows (low-entropy data).
//
// Boundaries match PlakarKorp/go-cdc-chunkers: the boundary index is the first
// byte of the next chunk. By default a boundary is reported at the exact byte
// that cleared the mask; WithSpecFaithful rounds boundaries up to the 8-byte
// window, matching plakar's "ultracdc-v1.0.0".
package ultracdc

import (
	"bytes"
	"io"
	"math/bits"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/chunkcore"
)

const (
	maskS                     uint64 = 0x2F
	maskL                     uint64 = 0x2C
	lowEntropyStringThreshold int    = 64
)

var hammingDistanceTo0xAA [256]int

func init() {
	for i := range hammingDistanceTo0xAA {
		hammingDistanceTo0xAA[i] = bits.OnesCount8(byte(i) ^ 0xAA)
	}
}

// A Chunker splits an io.Reader into content-defined chunks using UltraCDC.
//
//	c := ultracdc.New(r, minSize, normalSize, maxSize)
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
type Option func(*ucCut)

// WithSpecFaithful reports boundaries rounded up to the enclosing 8-byte window
// (cutpoint i+8) instead of the exact clearing byte (cutpoint i+j), matching
// plakar's "ultracdc-v1.0.0".
func WithSpecFaithful() Option {
	return func(f *ucCut) { f.specFaithful = true }
}

// WithBuffer supplies the working buffer, adopted when its capacity is large
// enough (roughly 2*maxSize). Use it to reuse one allocation across many
// streams.
func WithBuffer(buf []byte) Option {
	return func(f *ucCut) { f.buf = buf }
}

// New returns a Chunker over r. Chunk lengths are kept in [minSize, maxSize]
// with an average near normalSize.
func New(r io.Reader, minSize, normalSize, maxSize int, opts ...Option) *Chunker {
	f := &ucCut{min: minSize, normal: normalSize, max: maxSize}
	for _, opt := range opts {
		opt(f)
	}
	return &Chunker{core: chunkcore.New(r, f, f.buf)}
}

// ucCut is the UltraCDC cutpoint finder: a port of plakar
// chunkers/ultracdc/ultracdc.go Algorithm.
type ucCut struct {
	min          int
	normal       int
	max          int
	specFaithful bool
	buf          []byte
}

func (f *ucCut) MaxSize() int { return f.max }

// Window: UltraCDC compares 8-byte windows.
func (f *ucCut) Window() int { return 8 }

func (f *ucCut) Cut(data []byte, eof bool) (cutpoint int, contentDefined bool, sum uint64) {
	minSize, maxSize, normalSize := f.min, f.max, f.normal
	n := len(data)

	var lowEntropyCount int
	mask := maskS

	switch {
	case n <= minSize:
		return n, false, 0
	case n >= maxSize:
		n = maxSize
	case n <= normalSize:
		normalSize = n
	}

	if n < minSize+8 {
		return n, false, 0
	}

	// Hoist config and the lookup table so the window loop below touches no
	// struct fields and data[i] needs no bounds check.
	spec := f.specFaithful
	hd := &hammingDistanceTo0xAA
	data = data[:n:n]

	outBufWin := data[minSize : minSize+8]

	dist := 0
	for _, v := range outBufWin {
		dist += hd[v]
	}

	var inBufWin []byte
	for i := minSize + 8; i <= n-8; i += 8 {
		if i >= normalSize {
			mask = maskL
		}

		inBufWin = data[i : i+8]

		if bytes.Equal(inBufWin, outBufWin) {
			lowEntropyCount++
			if lowEntropyCount >= lowEntropyStringThreshold {
				return i + 8, true, uint64(dist)
			}
			continue
		}

		lowEntropyCount = 0
		for j := 0; j < 8; j++ {
			if (uint64(dist) & mask) == 0 {
				if spec {
					return i + 8, true, uint64(dist)
				}
				return i + j, true, uint64(dist)
			}
			dist += hd[data[i+j]] - hd[data[i+j-8]]
		}
		outBufWin = inBufWin
	}

	return n, false, 0
}

// Reset prepares the Chunker to split r from the start, reusing its buffers.
func (c *Chunker) Reset(r io.Reader) { c.core.Reset(r) }

// Next advances to the next chunk, returning false at end of input or on the
// first error.
func (c *Chunker) Next() bool { return c.core.Next() }

// Bytes returns the current chunk, valid until the next call to Next.
func (c *Chunker) Bytes() []byte { return c.core.Bytes() }

// ContentDefined reports whether the current chunk ended at a content-defined
// boundary (true) or was forced at max / end of stream (false).
func (c *Chunker) ContentDefined() bool { return c.core.ContentDefined() }

// Sum returns the running Hamming distance to 0xAA at the current chunk's
// content-defined boundary, or 0 for a forced cut. UltraCDC uses no rolling
// hash; this is the value its boundary mask is tested against.
func (c *Chunker) Sum() uint64 { return c.core.Sum() }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *Chunker) Offset() int { return c.core.Offset() }

// WindowSize returns 8: UltraCDC compares 8-byte windows.
func (c *Chunker) WindowSize() int { return c.core.WindowSize() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.core.Err() }
