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
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/cutcore"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/gearscan"
)

// cand is a candidate cut point kept on mxCut's monotonic stack: the Gear
// fingerprint at the position, and its offset relative to the current avail
// (rebased by each returned chunk length, mirroring buildbarn's discard).
type cand struct {
	hash uint64
	end  int
}

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
	core *cutcore.Core
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
	return &Chunker{core: cutcore.New(r, f, f.buf)}
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
	f.stack = make([]cand, 0, f.peek/f.min+2)
	return f
}

// mxCut is the MaxCDC cutpoint finder: a port of buildbarn/go-cdc's optimized
// maxChunkReader.ReadNextChunk. It keeps a monotonic stack of candidate cut
// points across calls so each input byte's Gear fingerprint is computed
// exactly once over the whole stream, rather than being rescanned by every
// chunk's horizon (which is what the naive port of simpleMaxChunkReader did,
// and why it ran ~1.3x slower than buildbarn's real implementation).
type mxCut struct {
	g    [256]uint64
	min  int
	peek int // min + max, the read-ahead buildbarn peeks per chunk
	buf  []byte

	// stack holds, oldest-first, the chunk boundaries not yet emitted: entry 0
	// is returned by the next Cut call, the rest carry forward. It mirrors
	// buildbarn's r.chunks minus its permanent zero-value anchor slot (dropped
	// here immediately after use instead of rebased to zero and kept).
	stack []cand
}

// ResetCut clears the carried-over candidate stack so a Reset'd Chunker
// starts identically to a fresh one.
func (f *mxCut) ResetCut() { f.stack = f.stack[:0] }

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
	// (>= 2*min) bytes at end of stream, so this is the EOF tail. The stack is
	// cleared since no further Cut call will need it rebased.
	if len(d) < 2*min {
		f.stack = f.stack[:0]
		return len(d), false, 0
	}

	// Reserve the trailing min bytes for the next chunk.
	d = d[:len(d)-min]

	g := &f.g
	var previous, current cand
	var older []cand
	if len(f.stack) >= 2 {
		// Resume where the previous call left off: current is the boundary
		// still being extended, previous is the last confirmed candidate
		// behind it, older is the rest of the stack (strictly increasing
		// hashes, oldest first).
		previous, current = f.stack[len(f.stack)-2], f.stack[len(f.stack)-1]
		older = append(f.stack[:0], f.stack[:len(f.stack)-2]...)
	} else {
		// First chunk of the stream (or the stack was cleared): seed the
		// fingerprint over the 64 bytes ending at the earliest legal cut.
		var h uint64
		for _, b := range d[min-window : min] {
			h = (h << 1) + g[b]
		}
		previous = cand{hash: h, end: min}
		current = previous
		older = f.stack[:0]
	}

	for {
		// Hash up to the next min-sized block boundary past current, or to
		// the end of the horizon, whichever comes first.
		region := d[current.end:]
		if m := min - (current.end - previous.end); len(region) > m {
			region = region[:m]
		}
		if len(region) == 0 {
			if current.end-previous.end == min {
				// No new maximum found in this block: it can never be
				// beaten by anything further out, so it's final.
				older = append(older, previous)
				previous = current
				continue
			}
			// Reached the horizon. previous/current (plus anything left in
			// older) are the stable stack for the next call; the oldest
			// entry is the chunk to emit now.
			f.stack = append(older, previous, current)
			break
		}
		// Four bytes per iteration: the recurrence is serial, but issuing the
		// four g[]-indexed loads as a straight-line group lets them pipeline
		// ahead of the dependent hash updates, the same trick gearscan.Max and
		// repmaxcdc's unrolled scan use. buildbarn's own maxChunkReader scans
		// scalar here; unrolling ours closes a further ~8% gap against it.
		i := 0
		for ; i+4 <= len(region); i += 4 {
			b := region[i : i+4 : i+4]
			g0, g1, g2, g3 := g[b[0]], g[b[1]], g[b[2]], g[b[3]]
			if h := (current.hash << 1) + g0; h > previous.hash {
				current.hash = h
				for len(older) > 0 && h > older[len(older)-1].hash {
					older = older[:len(older)-1]
				}
				previous = cand{hash: h, end: current.end + i + 1}
			} else {
				current.hash = h
			}
			if h := (current.hash << 1) + g1; h > previous.hash {
				current.hash = h
				for len(older) > 0 && h > older[len(older)-1].hash {
					older = older[:len(older)-1]
				}
				previous = cand{hash: h, end: current.end + i + 2}
			} else {
				current.hash = h
			}
			if h := (current.hash << 1) + g2; h > previous.hash {
				current.hash = h
				for len(older) > 0 && h > older[len(older)-1].hash {
					older = older[:len(older)-1]
				}
				previous = cand{hash: h, end: current.end + i + 3}
			} else {
				current.hash = h
			}
			if h := (current.hash << 1) + g3; h > previous.hash {
				current.hash = h
				for len(older) > 0 && h > older[len(older)-1].hash {
					older = older[:len(older)-1]
				}
				previous = cand{hash: h, end: current.end + i + 4}
			} else {
				current.hash = h
			}
		}
		for ; i < len(region); i++ {
			current.hash = (current.hash << 1) + g[region[i]]
			if current.hash > previous.hash {
				for len(older) > 0 && current.hash > older[len(older)-1].hash {
					older = older[:len(older)-1]
				}
				previous = cand{hash: current.hash, end: current.end + i + 1}
			}
		}
		current.end += len(region)
	}

	first := f.stack[0]
	for i := 1; i < len(f.stack); i++ {
		f.stack[i].end -= first.end
	}
	// Drop index 0 by copying the rest down onto the same backing array
	// (rather than reslicing f.stack[1:]), so the stack's capacity - and thus
	// this whole function's amortized zero-alloc behavior - isn't eaten one
	// element per chunk.
	f.stack = append(f.stack[:0], f.stack[1:]...)
	return first.end, true, first.hash
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
