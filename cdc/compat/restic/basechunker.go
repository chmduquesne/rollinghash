package restic

import (
	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/rabinkarp64"
)

// BaseChunker is restic/chunker's reader-less, incremental splitter: the caller
// pushes successive byte slices through NextSplitPoint and is told where each
// content-defined boundary falls, owning the chunk bytes itself. This is the
// API restic drives its file chunking through (internal/repository/chunker.go),
// so it is the one that matters for a drop-in replacement.
//
// Boundaries are byte-identical to restic/chunker.BaseChunker whenever MinSize
// is at least the 64-byte window. restic's 512 KiB default satisfies that, and
// a smaller-than-window MinSize misbehaves in restic/chunker too (unsigned
// underflow of its pre-skip counter), so this is not a real restriction.
//
// Unlike restic/chunker.BaseChunker, which scans the caller's slice in place,
// this implementation copies each byte once into an internal buffer (bounded by
// the larger of MaxSize and one read block). Reset with the same polynomial and
// no options, as restic does between files, keeps that buffer, so a chunker
// reused across a run of files allocates it once. On restic's own parameters
// (MinSize far above the average chunk, so roughly half the stream falls in the
// skipped pre-min region and is never hashed) this runs somewhat faster than
// restic/chunker; with small chunks, where per-chunk overhead dominates, it is
// somewhat slower.
type BaseChunker struct {
	pol     Pol
	min     uint
	max     uint
	avgBits int

	cw rollinghash.ChunkWriter

	// consumed is the caller's cursor: the stream offset of buf[0] on the next
	// NextSplitPoint call, advanced by each returned split and by each
	// fully-consumed buf. fed is the stream offset up to which bytes have been
	// handed to cw. fed >= consumed, and their difference is the part of the
	// caller's next buf that cw has already seen (restic re-presents the bytes
	// past a split), so it is never fed twice.
	consumed int
	fed      int
}

// baseOption configures a BaseChunker, mirroring restic/chunker's baseOption.
type baseOption func(*BaseChunker)

// WithBaseBoundaries sets custom min and max chunk-size bounds. It mirrors
// restic/chunker.WithBaseBoundaries.
func WithBaseBoundaries(min, max uint) baseOption {
	return func(c *BaseChunker) { c.min, c.max = min, max }
}

// WithBaseAverageBits sets the target average chunk size to 2^averageBits
// bytes. It mirrors restic/chunker.WithBaseAverageBits. The default is 20.
func WithBaseAverageBits(averageBits int) baseOption {
	return func(c *BaseChunker) { c.avgBits = averageBits }
}

// NewBase returns a BaseChunker that cuts with polynomial pol. It mirrors
// restic/chunker.NewBase. pol must be a non-zero polynomial of degree at most
// 53; NewBase panics otherwise (restic/chunker defers the equivalent panic to
// the first NextSplitPoint call).
func NewBase(pol Pol, opts ...baseOption) *BaseChunker {
	c := &BaseChunker{}
	c.init(pol, opts)
	return c
}

func (c *BaseChunker) init(pol Pol, opts []baseOption) {
	c.pol = pol
	c.min = MinSize
	c.max = MaxSize
	c.avgBits = defaultAverageBits
	for _, opt := range opts {
		opt(c)
	}
	if c.pol == 0 || c.pol.Deg() > 53 {
		panic("restic: polynomial must be non-zero and of degree <= 53 (use RandomPolynomial)")
	}
	h := rabinkarp64.NewFromPol(c.pol)
	mask := uint64(1)<<uint(c.avgBits) - 1
	c.cw = rollinghash.NewChunkWriter(h, windowSize, mask,
		rollinghash.WithBoundaries(int(c.min), int(c.max)))
	c.consumed = 0
	c.fed = 0
}

// Reset reinitialises the chunker to split a fresh stream from the start with
// polynomial pol. It mirrors restic/chunker.BaseChunker.Reset. Resetting with
// the same polynomial and no options, as restic does between files, keeps the
// internal buffer for reuse instead of reallocating it.
func (c *BaseChunker) Reset(pol Pol, opts ...baseOption) {
	if len(opts) == 0 && pol == c.pol && c.cw != nil {
		c.cw.Reset()
		c.consumed = 0
		c.fed = 0
		return
	}
	c.init(pol, opts)
}

// NextSplitPoint scans buf for the next chunk boundary. It returns the index
// within buf immediately after the boundary, so buf[:i] completes the current
// chunk and the caller re-presents buf[i:] on the following call. It returns -1
// when buf holds no boundary, in which case all of buf belongs to the current
// chunk. State carries across calls: every slice passed since the last split
// forms one chunk. It mirrors restic/chunker.BaseChunker.NextSplitPoint.
//
// One ChunkWriter runs for the whole stream, so each byte is hashed once and
// the cost is linear regardless of chunk size. It is never reset per chunk:
// with MinSize at or above the window the two implementations skip exactly the
// same pre-min region, so the window bytes that straddle a boundary are never
// evaluated and carrying them forward changes no boundary.
func (c *BaseChunker) NextSplitPoint(buf []byte) int {
	// buf covers stream offsets [consumed, consumed+len(buf)); feed only the
	// part cw has not already seen. Write copies, so the caller may reuse buf.
	if newTail := c.consumed + len(buf) - c.fed; newTail > 0 {
		fresh := buf[len(buf)-newTail:]
		c.cw.Write(fresh)
		c.cw.Flush()
		c.fed += newTail
	}

	if c.cw.Next() {
		// cw.Offset()+len(cw.Bytes()) is the boundary's stream offset; the
		// split is its distance from the caller's cursor.
		boundary := c.cw.Offset() + len(c.cw.Bytes())
		split := boundary - c.consumed
		c.consumed = boundary
		return split
	}

	c.consumed += len(buf)
	return -1
}
