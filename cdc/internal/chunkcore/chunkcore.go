// Package chunkcore is the shared streaming engine for the content-defined
// chunkers in the cdc tree (jumpchunker, fastcdc, ultracdc). It keeps a buffer
// holding at least one MaxSize window from the current chunk start and turns an
// algorithm-specific CutFinder into a Next/Bytes iterator, driven either by an
// io.Reader (New) or by Write/Close (NewWriter). It is internal and not part of
// the public API.
//
// The drive loop mirrors PlakarKorp/go-cdc-chunkers' Chunker.Next: make MaxSize
// bytes available (or reach EOF), ask the CutFinder for the next cut length,
// emit buf[start:start+n] (the boundary byte is excluded, it starts the next
// chunk), advance start.
//
// The buffer is not compacted on every chunk. start is a moving index into buf;
// consumed bytes are physically dropped only when buf runs out of tail room for
// the next read. The buffer is sized (2*MaxSize + compactionSlack) so that, by
// the time that happens, start has advanced well past MaxSize, making
// compaction move proportionally fewer bytes than it frees.
package chunkcore

import (
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
)

// readBlock is how many bytes Core pulls from the reader per fill.
const readBlock = 16 << 10

// compactionSlack is the spare tail room kept beyond the 2*MaxSize a full
// window needs. Compaction moves the live bytes (~MaxSize) every time the tail
// fills; more slack runs it less often and, since start has advanced further by
// then, each run copies a smaller fraction of what it reclaims. 16*readBlock
// (256 KiB) is the measured knee for a ~1 MiB working set — past it the buffer
// spills L2 and the residency costs more than the saved copies. On
// BenchmarkChunker this is ~+5% for jumpchunker (the most copy-bound algorithm)
// and neutral for fastcdc/ultracdc.
const compactionSlack = 16 * readBlock

// CutFinder is a stateless port of a plakar Algorithm function. It is called
// once per chunk with the bytes available from the current chunk start.
type CutFinder interface {
	// Cut returns the length of the next chunk, whether it ends at a
	// content-defined boundary (as opposed to a forced max cut or the final
	// bytes of the stream), and the algorithm's rolling value at that boundary.
	// The sum is used only when contentDefined is true; for a forced or final
	// cut the core recomputes it from WindowDigest. When eof is false, avail
	// holds at least MaxSize bytes. The returned length is clamped to
	// [1, len(avail)] by the caller.
	Cut(avail []byte, eof bool) (n int, contentDefined bool, sum uint64)
	// WindowDigest returns the algorithm's rolling value over exactly the bytes
	// b, which the core passes as the Window() bytes ending at a forced or
	// final cut so Sum is meaningful there too.
	WindowDigest(b []byte) uint64
	MaxSize() int
	// Window is the number of trailing bytes the boundary test depends on,
	// reported by Chunker.WindowSize.
	Window() int
}

// cutResetter is implemented by a CutFinder that carries state across Cut
// calls (an amortized search that must not leak into the next stream after
// Reset/ResetWriter).
type cutResetter interface {
	ResetCut()
}

// Core is the streaming buffer + iterator shared by the cdc chunkers.
type Core struct {
	r      io.Reader
	f      CutFinder
	max    int
	window int

	buf    []byte
	start  int // index in buf of the current (not yet emitted) chunk's first byte
	offset int // stream offset of buf[0]
	eof    bool
	done   bool
	err    error

	push   bool // fed by Write/Close instead of an io.Reader
	closed bool // Close has been called (push mode)

	chunk          []byte
	contentDefined bool
	sum            uint64
	curOff         int
}

// New returns a Core reading from r and cutting with f. If buf is non-nil and
// large enough (cap >= 2*MaxSize + compactionSlack) it is adopted as the
// working buffer; otherwise a fresh one is allocated.
func New(r io.Reader, f CutFinder, buf []byte) *Core {
	max := f.MaxSize()
	want := 2*max + compactionSlack
	if buf == nil || cap(buf) < want {
		buf = make([]byte, 0, want)
	}
	return &Core{
		r:      r,
		f:      f,
		max:    max,
		window: f.Window(),
		buf:    buf[:0],
	}
}

// NewWriter returns a push-mode Core: instead of pulling from an io.Reader it
// is fed via Write, and Close marks end of input. buf is adopted as in New.
func NewWriter(f CutFinder, buf []byte) *Core {
	c := New(nil, f, buf)
	c.push = true
	return c
}

// Reset prepares the Core to chunk r from the start, keeping the buffer alloc.
func (c *Core) Reset(r io.Reader) {
	c.r = r
	c.push = false
	c.resetState()
}

// ResetWriter clears a push-mode Core for reuse.
func (c *Core) ResetWriter() {
	c.r = nil
	c.push = true
	c.resetState()
}

func (c *Core) resetState() {
	if r, ok := c.f.(cutResetter); ok {
		r.ResetCut()
	}
	c.buf = c.buf[:0]
	c.start = 0
	c.offset = 0
	c.eof = false
	c.closed = false
	c.done = false
	c.err = nil
	c.chunk = nil
	c.contentDefined = false
	c.sum = 0
	c.curOff = 0
}

// Write appends p to the buffer for a later Next (push mode). It always
// consumes all of p and returns rollinghash.ErrClosed after Close. Callers
// should drain Next in a loop after each Write; the consumed prefix is dropped
// on demand, so the buffer stays bounded as long as that happens.
func (c *Core) Write(p []byte) (int, error) {
	if c.closed {
		return 0, rollinghash.ErrClosed
	}
	// Drop the consumed prefix when the tail can't hold p, so the buffer stays
	// bounded no matter how Write and Next are interleaved.
	if cap(c.buf)-len(c.buf) < len(p) {
		c.compact()
	}
	c.buf = append(c.buf, p...)
	return len(p), nil
}

// Close marks the end of input for a push-mode Core.
func (c *Core) Close() error {
	c.closed = true
	c.eof = true
	return nil
}

// Next advances to the next chunk. It returns false at end of input or on the
// first reader error (reported by Err).
func (c *Core) Next() bool {
	if c.err != nil || c.done {
		c.clearChunk()
		return false
	}

	// Make a full MaxSize window available from start, or reach EOF. In push
	// mode that means waiting for more Write calls; Next just reports "not
	// ready" by returning false without marking the Core done.
	if c.push {
		if len(c.buf)-c.start < c.max && !c.eof {
			c.clearChunk()
			return false
		}
	} else {
		for len(c.buf)-c.start < c.max && !c.eof {
			if !c.fill() {
				return c.fail()
			}
		}
	}

	avail := c.buf[c.start:]
	if len(avail) == 0 {
		c.done = true
		c.clearChunk()
		return false
	}

	n, cd, sum := c.f.Cut(avail, c.eof)
	switch {
	case n <= 0:
		n = 1 // never stall
	case n > len(avail):
		n = len(avail)
	}
	c.chunk = avail[:n]
	c.contentDefined = cd
	c.curOff = c.offset + c.start
	// Sum is the rolling value of the window ending at the cut, however the cut
	// was chosen. At a content-defined boundary that is the value Cut already
	// tested against the mask; at a forced or final cut it is recomputed from
	// the last window bytes (0 only when fewer than window bytes precede the
	// cut, i.e. a final chunk near the very start of the stream).
	if cd {
		c.sum = sum
	} else {
		c.sum = c.windowDigestAt(c.start + n)
	}
	c.start += n
	if c.eof && c.start >= len(c.buf) {
		c.done = true
	}
	return true
}

func (c *Core) clearChunk() {
	c.chunk = nil
	c.contentDefined = false
	c.sum = 0
}

// windowDigestAt returns the finder's rolling value over the window of c.window
// bytes ending just before buf index end (exclusive). It returns 0 when fewer
// than c.window bytes precede end; compact retains c.window-1 bytes of lead-in
// before c.start, so this happens only for a final chunk lying within c.window
// bytes of the start of the stream.
func (c *Core) windowDigestAt(end int) uint64 {
	lo := end - c.window
	if lo < 0 {
		return 0
	}
	return c.f.WindowDigest(c.buf[lo:end])
}

// compact drops the consumed prefix of buf, retaining c.window-1 bytes of
// lead-in before start so windowDigestAt can still see a window that straddles
// the previous chunk's end.
func (c *Core) compact() {
	keep := min(c.start, c.window-1)
	src := c.start - keep
	if src == 0 {
		return
	}
	m := copy(c.buf, c.buf[src:])
	c.buf = c.buf[:m]
	c.offset += src
	c.start = keep
}

// fill drops the consumed prefix if buf has no tail room, then reads one
// readBlock into buf's tail. It returns false on reader error (with err set);
// io.EOF sets eof and is not an error.
func (c *Core) fill() bool {
	if cap(c.buf)-len(c.buf) < readBlock {
		c.compact()
		if cap(c.buf)-len(c.buf) < readBlock { // still tight: grow (shouldn't happen given New's sizing)
			grown := make([]byte, len(c.buf), len(c.buf)+readBlock)
			copy(grown, c.buf)
			c.buf = grown
		}
	}
	base := len(c.buf)
	c.buf = c.buf[:base+readBlock]
	nread := 0
	for nread < readBlock && !c.eof {
		m, err := c.r.Read(c.buf[base+nread : base+readBlock])
		nread += m
		switch {
		case err == io.EOF:
			c.eof = true
		case err != nil:
			c.err = err
			c.buf = c.buf[:base+nread]
			return false
		}
	}
	c.buf = c.buf[:base+nread]
	return true
}

func (c *Core) fail() bool {
	c.done = true
	c.clearChunk()
	return false
}

// Bytes returns the current chunk, aliasing the internal buffer; it is valid
// only until the next call to Next. It is nil before the first Next and after
// Next returns false.
func (c *Core) Bytes() []byte { return c.chunk }

// ContentDefined reports whether the current chunk ended at a content-defined
// boundary (as opposed to a forced max cut or the final bytes of the stream).
func (c *Core) ContentDefined() bool { return c.contentDefined }

// Sum returns the algorithm's rolling value for the window ending at the current
// chunk's cut, whether that cut was a mask hit, a forced cut at max, or the end
// of the stream. At a content-defined boundary it is the value the mask was
// tested against. It is 0 only for a final chunk lying within WindowSize bytes
// of the start of the stream, and before the first Next / after Next returns
// false.
func (c *Core) Sum() uint64 { return c.sum }

// Offset is the stream offset of the current chunk's first byte.
func (c *Core) Offset() int { return c.curOff }

// WindowSize is the number of trailing bytes the boundary test depends on.
func (c *Core) WindowSize() int { return c.window }

// Err returns the first non-EOF reader error, if any.
func (c *Core) Err() error { return c.err }
