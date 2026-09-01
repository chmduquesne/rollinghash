// Package jumpchunker splits a stream into content-defined chunks using the
// Jump Chunking (JC) algorithm.
//
// JC uses a windowless accumulating Gear fingerprint (fp = (fp<<1) + G[b]) with
// a dual-mask trick: a wider mask maskJ is tested first and, on a miss, the scan
// jumps jumpLen bytes ahead instead of stepping one byte, skipping regions that
// provably cannot hold a boundary. This trades different boundary positions for
// higher throughput than the parent package's rolling-window Chunker.
//
// Boundaries match PlakarKorp/go-cdc-chunkers: the boundary byte is the first
// byte of the next chunk, not the last byte of the current one.
package jumpchunker

import (
	"io"
	"math/bits"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/chunkcore"
)

// gearTabler is the capability jumpchunker needs from its hash: read access to
// the 256-entry Gear table so the JC fingerprint loop can run. gearhash64
// satisfies it.
type gearTabler interface {
	Table() [256]uint64
}

// A Chunker splits an io.Reader into content-defined chunks using Jump Chunking.
//
//	c := jumpchunker.New(r, gearhash64.New(), normalSize, min, max)
//	for c.Next() {
//		chunk := c.Bytes()
//		if c.AtMask() {
//			// content-defined boundary
//		} else {
//			// forced cut at max, or the final chunk at end of stream
//		}
//	}
//	if err := c.Err(); err != nil { ... }
//
// The hash must expose Table(); New panics otherwise. Use Reset to reuse the
// Chunker across streams without extra allocations.
type Chunker struct {
	core *chunkcore.Core
}

// Option configures New.
type Option func(*jcCut)

// WithJumpMask overrides the maskC and jumpLen that New would otherwise derive
// from normalSize. maskJ is recomputed as maskC & (maskC-1). Use this to match
// another implementation that fixes its own boundary mask and jump stride (for
// example plakar's legacy "jc": maskC 0x590003570000, jumpLen 4096).
func WithJumpMask(maskC uint64, jumpLen int) Option {
	return func(f *jcCut) {
		f.maskC = maskC
		f.maskJ = maskC & (maskC - 1)
		f.jumpLen = jumpLen
	}
}

// WithSpecFaithful selects the paper's Algorithm 1 behaviour: a final segment
// shorter than normalSize is still scanned for a boundary. Without it (the
// default, matching plakar's "jc"/"jc-v1.0.0"), such a tail is emitted whole.
func WithSpecFaithful() Option {
	return func(f *jcCut) { f.specFaithful = true }
}

// WithBuffer supplies the working buffer, adopted when its capacity is large
// enough (roughly 2*max). Use it to reuse one allocation across many streams.
func WithBuffer(buf []byte) Option {
	return func(f *jcCut) { f.buf = buf }
}

// New returns a Chunker over r. normalSize is the target average chunk length;
// maskC and jumpLen are derived from it. Chunk lengths are kept in [min, max].
// h must expose Table() (gearhash64 does); New panics otherwise.
func New(r io.Reader, h rollinghash.Hash, normalSize, min, max int, opts ...Option) *Chunker {
	ht, ok := h.(gearTabler)
	if !ok {
		panic("jumpchunker: Chunker requires a Gear hash exposing Table()")
	}
	maskC, jumpLen := jumpParams(normalSize)
	f := &jcCut{
		g:       ht.Table(),
		maskC:   maskC,
		maskJ:   maskC & (maskC - 1),
		jumpLen: jumpLen,
		min:     min,
		normal:  normalSize,
		max:     max,
	}
	for _, opt := range opts {
		opt(f)
	}
	return &Chunker{core: chunkcore.New(r, f, f.buf)}
}

// jumpParams derives maskC and jumpLen for a target normalSize (this package's
// own tuning, independent of plakar; use WithJumpMask for interop).
//
// bits = floor(log2(normalSize)); cOnes = bits-2 set bits in maskC;
// jumpLen = 2^(bits-1). This gives a 1/5 byte-examination rate.
func jumpParams(normalSize int) (maskC uint64, jumpLen int) {
	lg := bits.Len(uint(normalSize)) - 1
	if lg < 3 {
		lg = 3
	}
	cOnes := lg - 2
	jumpLen = 1 << (lg - 1)
	maskC = jumpMask(cOnes)
	return
}

// jumpMask builds a uint64 with exactly cOnes set bits, evenly spaced from bit
// 63 downward with step = 64/cOnes.
func jumpMask(cOnes int) uint64 {
	step := 64 / cOnes
	var mask uint64
	for i := 0; i < cOnes; i++ {
		mask |= 1 << uint(63-i*step)
	}
	return mask
}

// jcCut is the JC cutpoint finder: a port of plakar chunkers/jc/jc.go Algorithm.
type jcCut struct {
	g            [256]uint64
	maskC        uint64
	maskJ        uint64
	jumpLen      int
	min          int
	normal       int
	max          int
	specFaithful bool
	buf          []byte
}

func (f *jcCut) MaxSize() int { return f.max }

func (f *jcCut) Cut(data []byte, eof bool) (int, bool) {
	n := len(data)

	switch {
	case f.specFaithful:
		if n >= f.max {
			n = f.max
		}
	case n <= f.normal:
		return n, false
	case n >= f.max:
		n = f.max
	}

	// Hoist config into locals so the scan loop below does not reload struct
	// fields through the receiver pointer on every iteration.
	g := &f.g
	maskC, maskJ, jumpLen := f.maskC, f.maskJ, f.jumpLen
	data = data[:n:n] // fold the bound into data so data[i] needs no separate check

	fp := uint64(0)
	for i := f.min; i < n; {
		fp = (fp << 1) + g[data[i]]
		if fp&maskJ == 0 {
			if fp&maskC == 0 {
				return i, true // boundary byte i starts the next chunk
			}
			fp = 0
			i += jumpLen
		} else {
			i++
		}
	}
	return n, false
}

// Reset prepares the Chunker to split r from the start, reusing its buffers.
func (c *Chunker) Reset(r io.Reader) { c.core.Reset(r) }

// Next advances to the next chunk, returning false at end of input or on the
// first error. After it returns false, Err reports any error other than EOF.
func (c *Chunker) Next() bool { return c.core.Next() }

// Bytes returns the current chunk, valid until the next call to Next. Before the
// first call to Next, and after Next returns false, Bytes returns nil.
func (c *Chunker) Bytes() []byte { return c.core.Bytes() }

// AtMask reports whether the current chunk was cut by the mask (true) or forced
// at max / end of stream (false).
func (c *Chunker) AtMask() bool { return c.core.AtMask() }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *Chunker) Offset() int { return c.core.Offset() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.core.Err() }
