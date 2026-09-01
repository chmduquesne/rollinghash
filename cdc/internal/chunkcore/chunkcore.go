// Package chunkcore is the shared streaming engine for the content-defined
// chunkers in the cdc tree (jumpchunker, fastcdc, ultracdc). It owns the
// io.Reader, keeps a buffer holding at least one MaxSize window from the
// current chunk start, and turns an algorithm-specific CutFinder into a
// Next/Bytes iterator. It is internal and not part of the public API.
//
// The drive loop mirrors PlakarKorp/go-cdc-chunkers' Chunker.Next: make MaxSize
// bytes available (or reach EOF), ask the CutFinder for the next cut length,
// emit buf[start:start+n] (the boundary byte is excluded, it starts the next
// chunk), advance start.
//
// The buffer is not compacted on every chunk. start is a moving index into buf;
// consumed bytes are physically dropped only when buf runs out of tail room for
// the next read. The buffer is sized (2*MaxSize + slack) so that, by the time
// that happens, start has advanced ~MaxSize, making compaction amortize to
// roughly one moved byte per output byte instead of one per chunk.
package chunkcore

import "io"

// readBlock is how many bytes Core pulls from the reader per fill.
const readBlock = 16 << 10

// CutFinder is a stateless port of a plakar Algorithm function. It is called
// once per chunk with the bytes available from the current chunk start.
type CutFinder interface {
	// Cut returns the length of the next chunk, whether it ends at a
	// content-defined boundary (as opposed to a forced max cut or the final
	// bytes of the stream), and the algorithm's rolling value at that boundary
	// (0 when contentDefined is false). When eof is false, avail holds at
	// least MaxSize bytes. The returned length is clamped to [1, len(avail)]
	// by the caller.
	Cut(avail []byte, eof bool) (n int, contentDefined bool, sum uint64)
	MaxSize() int
	// Window is the number of trailing bytes the boundary test depends on,
	// reported by Chunker.WindowSize.
	Window() int
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

	chunk          []byte
	contentDefined bool
	sum            uint64
	curOff         int
}

// New returns a Core reading from r and cutting with f. If buf is non-nil and
// large enough (cap >= 2*MaxSize + 2*readBlock) it is adopted as the working
// buffer; otherwise a fresh one is allocated.
func New(r io.Reader, f CutFinder, buf []byte) *Core {
	max := f.MaxSize()
	want := 2*max + 2*readBlock
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

// Reset prepares the Core to chunk r from the start, keeping the buffer alloc.
func (c *Core) Reset(r io.Reader) {
	c.r = r
	c.buf = c.buf[:0]
	c.start = 0
	c.offset = 0
	c.eof = false
	c.done = false
	c.err = nil
	c.chunk = nil
	c.contentDefined = false
	c.sum = 0
	c.curOff = 0
}

// Next advances to the next chunk. It returns false at end of input or on the
// first reader error (reported by Err).
func (c *Core) Next() bool {
	if c.err != nil || c.done {
		c.clearChunk()
		return false
	}

	// Make a full MaxSize window available from start, or reach EOF.
	for len(c.buf)-c.start < c.max && !c.eof {
		if !c.fill() {
			return c.fail()
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
	c.sum = sum
	c.curOff = c.offset + c.start
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

// fill drops the consumed prefix if buf has no tail room, then reads one
// readBlock into buf's tail. It returns false on reader error (with err set);
// io.EOF sets eof and is not an error.
func (c *Core) fill() bool {
	if cap(c.buf)-len(c.buf) < readBlock {
		if c.start > 0 {
			m := copy(c.buf, c.buf[c.start:])
			c.buf = c.buf[:m]
			c.offset += c.start
			c.start = 0
		}
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

// Sum returns the algorithm's rolling value at the current chunk's boundary, or
// 0 when the chunk was not cut at a content-defined boundary.
func (c *Core) Sum() uint64 { return c.sum }

// Offset is the stream offset of the current chunk's first byte.
func (c *Core) Offset() int { return c.curOff }

// WindowSize is the number of trailing bytes the boundary test depends on.
func (c *Core) WindowSize() int { return c.window }

// Err returns the first non-EOF reader error, if any.
func (c *Core) Err() error { return c.err }
