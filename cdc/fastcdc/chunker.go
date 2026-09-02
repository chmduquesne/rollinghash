// Package fastcdc splits a stream into content-defined chunks using the FastCDC
// algorithm (Xia et al., 2016): a windowless accumulating Gear fingerprint with
// normalized chunking (a stricter mask below the target size, a looser one
// above) and cut-point skipping (the first MinSize bytes of every chunk are not
// scanned).
//
// Boundaries match PlakarKorp/go-cdc-chunkers: the boundary byte is the first
// byte of the next chunk. By default the boundary masks are derived from
// normalSize; use WithMasks to pin them (LegacyMaskS/LegacyMaskL reproduce
// plakar's "fastcdc" variant and the reference implementation's 8 KiB tuning).
//
// New returns a pull-based *Chunker (rollinghash.Chunker) over an io.Reader;
// NewChunkWriter returns the push-based *ChunkWriter (rollinghash.ChunkWriter),
// fed via Write/Close.
//
// Keyed chunking: users who want this can pass any table (via
// gearhash64.NewFromUint64Array) This package provides no key-derivation
// helper; derive the table however you like. To reproduce plakar's "kfastcdc"
// look at cdc/compat/gocdc/bench/keyed_test.go, function plakarKeyedGearTable
package fastcdc

import (
	"io"
	"math"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/chunkcore"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/gearscan"
)

// LegacyMaskS and LegacyMaskL are the fixed masks the reference FastCDC uses at
// its 2 KiB / 8 KiB / 64 KiB tuning, and that plakar's "fastcdc" variant uses
// unconditionally.
const (
	LegacyMaskS = uint64(0x0003590703530000)
	LegacyMaskL = uint64(0x0000d90003530000)
)

// normalLevel is the bit-count offset between the target mask and the strict /
// loose masks. FastCDC fixes it at 2.
const normalLevel = 2

type gearTabler interface {
	Table() [256]uint64
}

// A Chunker splits an io.Reader into content-defined chunks using FastCDC.
//
//	c := fastcdc.New(r, gearhash64.New(), minSize, normalSize, maxSize)
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
type Option func(*fcCut)

// WithMasks overrides the maskS/maskL that New would otherwise derive from
// normalSize. maskS is used before the chunk reaches normalSize, maskL at or
// after it. A mask with more set bits fires less often (longer chunks).
func WithMasks(maskS, maskL uint64) Option {
	return func(f *fcCut) { f.maskS, f.maskL = maskS, maskL }
}

// WithBuffer supplies the working buffer, adopted when its capacity is large
// enough (roughly 2*maxSize). Use it to reuse one allocation across many
// streams.
func WithBuffer(buf []byte) Option {
	return func(f *fcCut) { f.buf = buf }
}

// New returns a Chunker over r. Chunk lengths are kept in [minSize, maxSize]
// with an average near normalSize. h must expose Table(); New panics otherwise.
func New(r io.Reader, h rollinghash.Hash, minSize, normalSize, maxSize int, opts ...Option) *Chunker {
	f := newCut(h, minSize, normalSize, maxSize, opts)
	return &Chunker{core: chunkcore.New(r, f, f.buf)}
}

func newCut(h rollinghash.Hash, minSize, normalSize, maxSize int, opts []Option) *fcCut {
	ht, ok := h.(gearTabler)
	if !ok {
		panic("fastcdc: requires a Gear hash exposing Table()")
	}
	maskS, maskL := calculateMasks(normalSize, normalLevel)
	f := &fcCut{
		g:      ht.Table(),
		maskS:  maskS,
		maskL:  maskL,
		min:    minSize,
		normal: normalSize,
		max:    maxSize,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// calculateMasks derives the strict and loose masks for a target size, matching
// the FastCDC reference (and plakar's non-legacy path).
func calculateMasks(normalSize, level int) (maskS, maskL uint64) {
	b := uint64(math.Log2(float64(normalSize)))
	maskS = generateSpacedMask(int(b+uint64(level)), 64)
	maskL = generateSpacedMask(int(b-uint64(level)), 64)
	return
}

// generateSpacedMask builds a uint64 with oneCount set bits spread evenly from
// the top of the word down.
func generateSpacedMask(oneCount, totalBits int) uint64 {
	if oneCount >= totalBits {
		return 0xFFFFFFFFFFFFFFFF
	}
	if oneCount <= 0 {
		return 0
	}
	step := totalBits / oneCount
	var mask uint64
	for i := 0; i < oneCount; i++ {
		if pos := totalBits - 1 - i*step; pos >= 0 {
			mask |= 1 << pos
		}
	}
	return mask
}

// fcCut is the FastCDC cutpoint finder: a port of plakar
// chunkers/fastcdc/fastcdc.go Algorithm.
type fcCut struct {
	g            [256]uint64
	maskS, maskL uint64
	min          int
	normal       int
	max          int
	buf          []byte
}

func (f *fcCut) MaxSize() int { return f.max }

// Window: the Gear fingerprint is windowless but a 64-bit accumulator retains
// only the last 64 bytes.
func (f *fcCut) Window() int { return 64 }

func (f *fcCut) Cut(data []byte, eof bool) (int, bool, uint64) {
	n := len(data)
	normalSize := f.normal

	switch {
	case n <= f.min:
		return n, false, 0
	case n >= f.max:
		n = f.max
	case n <= normalSize:
		normalSize = n
	}

	// Hoist config into locals and split the scan at normalSize so neither
	// loop reloads struct fields or re-tests the mask-switch per byte.
	// Phase 1 uses the strict mask up to normalSize, phase 2 the loose mask
	// after it, reproducing plakar's "switch when i == normalSize" exactly
	// (including the edge cases normalSize <= f.min).
	g := &f.g
	maskS, maskL := f.maskS, f.maskL
	data = data[:n:n]

	i := f.min
	var p1end int
	switch {
	case normalSize > i:
		p1end = normalSize
	case normalSize == i:
		p1end = i // phase 1 empty; phase 2 (loose mask) covers everything
	default: // normalSize < i: the switch never fires, strict mask throughout
		p1end = n
	}

	// Phase 1: strict mask over [i, p1end). Phase 2: loose mask over [p1end, n),
	// continuing the same fingerprint. The boundary byte starts the next chunk.
	pos, hit, fp := gearscan.Scan4(g, data, i, p1end, maskS, 0)
	if hit {
		return pos, true, fp
	}
	if pos, hit, fp = gearscan.Scan4(g, data, p1end, n, maskL, fp); hit {
		return pos, true, fp
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

// ContentDefined reports whether the current chunk was cut by a mask hit (true)
// or forced at max / end of stream (false).
func (c *Chunker) ContentDefined() bool { return c.core.ContentDefined() }

// Sum returns the Gear fingerprint at the current chunk's content-defined
// boundary, or 0 for a forced cut.
func (c *Chunker) Sum() uint64 { return c.core.Sum() }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *Chunker) Offset() int { return c.core.Offset() }

// WindowSize returns 64: the Gear fingerprint is windowless, but a 64-bit
// accumulator retains only the last 64 bytes.
func (c *Chunker) WindowSize() int { return c.core.WindowSize() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.core.Err() }
