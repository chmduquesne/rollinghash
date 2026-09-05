// Package repmaxcdc splits a stream into content-defined chunks using RepMaxCDC
// ("repeated maximum"), the tight-bound chunker from buildbarn/go-cdc that is
// one of the standard CDC functions proposed for Bazel's remote-execution
// protocol.
//
// It builds on MaxCDC: over a windowless accumulating Gear fingerprint
// (h = (h<<1) + gear[b], a 64-byte effective window) it looks for the position
// in a read-ahead horizon where the fingerprint is maximal, and cuts there.
// RepMaxCDC then repeats that search whenever the chosen maximum would make the
// chunk too large, restricting the horizon to just before it, until every chunk
// lands in [minSize, 2*minSize). The strict size bound makes it trivial to tell
// from its length alone whether a blob is already chunked.
//
// Parameters are minSize and a horizon size (unlike the min/normal/max of
// fastcdc and ultracdc): the horizon only controls chunking quality — a cut is
// always at least as good as the best position within
// [minSize, minSize+horizon] — and can be raised freely, with diminishing
// returns. The maximum chunk size is fixed at 2*minSize (exclusive).
//
// Boundaries match buildbarn/go-cdc's NewRepMaxContentDefinedChunker /
// NewSimpleRepMaxContentDefinedChunker for the same Gear table, minSize and
// horizon. The Gear table is supplied by the caller through the hash passed to
// New (any hash exposing Table(); gearhash64 does), so keyed chunking is just a
// keyed table. buildbarn's own default is the well-known FastCDC gear table
// (nlfiedler/fastcdc-rs, buildbuddy-io/fastcdc2020); pass that same table to
// reproduce its output.
//
// New returns a pull-based *Chunker (rollinghash.Chunker) over an io.Reader;
// NewChunkWriter returns the push-based *ChunkWriter (rollinghash.ChunkWriter),
// fed via Write/Close.
package repmaxcdc

import (
	"io"
	"slices"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/cutcore"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/gearscan"
)

// window is the number of trailing bytes the Gear fingerprint depends on. The
// fingerprint is windowless but its 64-bit accumulator retains only the last 64
// bytes, and RepMaxCDC seeds each search from exactly this many bytes.
const window = 64

type gearTabler interface {
	Table() [256]uint64
}

// A Chunker splits an io.Reader into content-defined chunks using RepMaxCDC.
//
//	c := repmaxcdc.New(r, gearhash64.New(), minSize, horizon)
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
type Option func(*rmCut)

// WithBuffer supplies the working buffer, adopted when its capacity is large
// enough (roughly 2*(2*minSize+horizon)). Use it to reuse one allocation across
// many streams.
func WithBuffer(buf []byte) Option {
	return func(f *rmCut) { f.buf = buf }
}

// New returns a Chunker over r. Chunk lengths are kept in [minSize, 2*minSize);
// horizon is the read-ahead within which the repeated-maximum search always
// finds the optimal cut (0 gives uniform minSize chunks). h must expose
// Table(); New panics otherwise, or if minSize < 64 or horizon < 0.
func New(r io.Reader, h rollinghash.Hash, minSize, horizon int, opts ...Option) *Chunker {
	f := newCut(h, minSize, horizon, opts)
	return &Chunker{core: cutcore.New(r, f, f.buf)}
}

func newCut(h rollinghash.Hash, minSize, horizon int, opts []Option) *rmCut {
	ht, ok := h.(gearTabler)
	if !ok {
		panic("repmaxcdc: requires a Gear hash exposing Table()")
	}
	if minSize < window {
		panic("repmaxcdc: minSize must be >= 64 (the Gear window)")
	}
	if horizon < 0 {
		panic("repmaxcdc: horizon must be >= 0")
	}
	f := &rmCut{
		g:    ht.Table(),
		min:  minSize,
		peek: 2*minSize + horizon,
	}
	for _, opt := range opts {
		opt(f)
	}
	f.completeChunks = make([]int, 0, f.peek/f.min+1)
	// The incomplete-chunks stack can in principle grow with the horizon, but
	// in practice it becomes exponentially harder to find an even better
	// cutting point as the search progresses; 32 covers virtually all inputs
	// (matching buildbarn's own sizing).
	f.incompleteChunks = make([]int, 0, 32)
	return f
}

// rmCut is the RepMaxCDC cutpoint finder: a port of buildbarn/go-cdc's
// optimized repMaxChunkReader.ReadNextChunk. It keeps two stacks across Cut
// calls - completeChunks (boundaries no future data can move) and
// incompleteChunks (candidate cutting points with strictly increasing Gear
// fingerprints) - so each input byte is hashed exactly once over the whole
// stream, rather than the horizon being rescanned by every restart of the
// repeated-maximum search (which is what the naive port of
// simpleRepMaxChunkReader did, and why it ran ~1.4x slower than buildbarn's
// real implementation).
type rmCut struct {
	g    [256]uint64
	min  int
	peek int // 2*min + horizon, the read-ahead buildbarn peeks per chunk
	buf  []byte

	// completeChunks holds, in reverse order (so the next chunk to emit is
	// the last element), the lengths of chunks no more data can influence.
	completeChunks []int
	// incompleteChunks holds candidate cutting points, offset relative to the
	// first eligible position (last complete chunk's end + min); entry 0 is
	// always 0, and the Gear fingerprints at these offsets strictly increase.
	incompleteChunks []int
	// currentHash is the running fingerprint up to the position processing
	// has reached; bestHash is the fingerprint of the last (best) incomplete
	// chunk, i.e. incompleteChunks[len-1].
	currentHash, bestHash uint64
}

// ResetCut clears the carried-over search state so a Reset'd Chunker starts
// identically to a fresh one.
func (f *rmCut) ResetCut() {
	f.completeChunks = f.completeChunks[:0]
	f.incompleteChunks = f.incompleteChunks[:0]
	f.currentHash, f.bestHash = 0, 0
}

func (f *rmCut) MaxSize() int { return f.peek }

// Window: the Gear fingerprint is windowless but its accumulator retains only
// the last 64 bytes.
func (f *rmCut) Window() int { return window }

// WindowDigest returns the Gear fingerprint of b, used for Sum at a forced or
// final cut.
func (f *rmCut) WindowDigest(b []byte) uint64 { return gearscan.Digest(&f.g, b) }

func (f *rmCut) Cut(d []byte, eof bool) (int, bool, uint64) {
	min := f.min

	// Fast path: an earlier horizon scan already produced more than one
	// stable boundary. Hand them out one at a time with no further scanning,
	// so the core can discard as aggressively as possible in between.
	if n := len(f.completeChunks); n > 0 {
		size := f.completeChunks[n-1]
		f.completeChunks = f.completeChunks[:n-1]
		return size, true, f.WindowDigest(d[size-window : size])
	}

	// Cap the view to the peek horizon, matching buildbarn's
	// Peek(peekSizeBytes). The core may hand us more than MaxSize.
	if len(d) > f.peek {
		d = d[:f.peek]
	}

	// Too little left to guarantee a >= min follow-up chunk: emit everything
	// as one forced chunk. The core only calls Cut with fewer than MaxSize
	// (>= 2*min) bytes at end of stream, so this is the EOF tail.
	if len(d) < 2*min {
		f.completeChunks = f.completeChunks[:0]
		f.incompleteChunks = f.incompleteChunks[:0]
		return len(d), false, 0
	}

	// Reserve the trailing min bytes for the next chunk.
	d = d[:len(d)-min]

	g := &f.g
	completeChunks := f.completeChunks[:0]
	var oldChunks []int
	var currentChunk int
	var currentHash, bestHash uint64
	if len(f.incompleteChunks) >= 2 {
		// Resume where the previous call left off.
		oldChunks = f.incompleteChunks[:len(f.incompleteChunks)-1]
		currentChunk = f.incompleteChunks[len(f.incompleteChunks)-1]
		currentHash, bestHash = f.currentHash, f.bestHash
	} else {
		// First chunk of the stream (or the state was cleared): the first
		// min positions can't contain a cut, so seed over them and start the
		// stack at offset 0.
		oldChunks = append(f.incompleteChunks[:0], 0)
		for _, b := range d[min-window : min] {
			currentHash = (currentHash << 1) + g[b]
		}
		bestHash = currentHash
	}

	// Extend the candidate stack across the horizon: hash forward, pushing
	// every new record; whenever a full min-sized block passes with no new
	// record, everything up to min bytes behind the stack's tail is final and
	// can be popped off into completeChunks (possibly several chunks at
	// once, since a long stable run collapses in one step).
	uncompletedRegion := d[min+currentChunk:]
	for {
		hashRegion := uncompletedRegion
		originalOldChunksCount := -1
		if bytesBeforeMinChunkSize := oldChunks[len(oldChunks)-1] + min - 1 - currentChunk; len(hashRegion) > bytesBeforeMinChunkSize {
			hashRegion = hashRegion[:bytesBeforeMinChunkSize]
			originalOldChunksCount = len(oldChunks)
		} else if len(hashRegion) == 0 {
			break
		}

		// Eight bytes per iteration: the recurrence is serial, but issuing the
		// eight dependent gear[]/shift steps as a straight-line group lets
		// them pipeline instead of stalling behind the branch each turn (this
		// is buildbarn's own unrolling, "empirically determined to give good
		// performance" - a scalar version of this loop was ~1.5x slower).
		i := 0
		for ; i+8 <= len(hashRegion); i += 8 {
			b := hashRegion[i : i+8 : i+8]
			s := g[b[0]]
			h := (currentHash << 1) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+1)
			}
			s = (s << 1) + g[b[1]]
			h = (currentHash << 2) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+2)
			}
			s = (s << 1) + g[b[2]]
			h = (currentHash << 3) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+3)
			}
			s = (s << 1) + g[b[3]]
			h = (currentHash << 4) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+4)
			}
			s = (s << 1) + g[b[4]]
			h = (currentHash << 5) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+5)
			}
			s = (s << 1) + g[b[5]]
			h = (currentHash << 6) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+6)
			}
			s = (s << 1) + g[b[6]]
			h = (currentHash << 7) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+7)
			}
			s = (s << 1) + g[b[7]]
			h = (currentHash << 8) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+8)
			}
			currentHash = h
		}
		for ; i < len(hashRegion); i++ {
			currentHash = (currentHash << 1) + g[hashRegion[i]]
			if bestHash < currentHash {
				bestHash = currentHash
				oldChunks = append(oldChunks, currentChunk+i+1)
			}
		}

		if len(oldChunks) == originalOldChunksCount {
			// No new record turned up, and we're exactly min bytes past the
			// stack's tail: everything up to it is now final.
			previousCompleteChunksCount := len(completeChunks)
			nextChunk := oldChunks[len(oldChunks)-1]
			for i := len(oldChunks) - 3; nextChunk >= min; i-- {
				chunk := oldChunks[i]
				if sizeBytes := nextChunk - chunk; sizeBytes >= min {
					completeChunks = append(completeChunks, sizeBytes)
					nextChunk = chunk
					i--
				}
			}
			completeChunks = append(completeChunks, min+nextChunk)
			slices.Reverse(completeChunks[previousCompleteChunksCount:])

			oldChunks = oldChunks[:1]
			currentChunk = 0
			currentHash = (currentHash << 1) + g[uncompletedRegion[len(hashRegion)]]
			bestHash = currentHash
			uncompletedRegion = uncompletedRegion[len(hashRegion)+1:]
		} else {
			currentChunk += len(hashRegion)
			uncompletedRegion = uncompletedRegion[len(hashRegion):]
		}
	}

	// Processed the full horizon. oldChunks+currentChunk is the stable stack
	// carried into the next call; determine which chunk to emit now.
	incompleteChunks := append(oldChunks, currentChunk)
	var firstChunk int
	if len(completeChunks) > 0 {
		slices.Reverse(completeChunks)
		firstChunk = completeChunks[len(completeChunks)-1]
		completeChunks = completeChunks[:len(completeChunks)-1]
	} else {
		// The horizon scan didn't naturally complete anything - either we hit
		// end of stream, or the horizon wasn't large enough. Pick a cutting
		// point that still respects the maximum chunk size while remaining
		// the most preferable one available.
		firstChunkIndex := len(incompleteChunks) - 2
		for maxChunk, i := incompleteChunks[firstChunkIndex]-min, firstChunkIndex-2; maxChunk >= 0; i-- {
			if chunk := incompleteChunks[i]; chunk <= maxChunk {
				firstChunkIndex = i
				maxChunk = chunk - min
				i--
			}
		}
		firstChunk = min + incompleteChunks[firstChunkIndex]

		// Cutting points after the selected one that are no longer eligible
		// (they'd violate the minimum chunk size) must be dropped; keep the
		// rest, recomputing any that were glossed over if the first one kept
		// wasn't already offset 0 relative to the new chunk start.
		reusableChunkIndex := firstChunkIndex + 1
		for {
			if offsetInSecondChunk := incompleteChunks[reusableChunkIndex] - firstChunk; offsetInSecondChunk >= 0 {
				for i := reusableChunkIndex; i < len(incompleteChunks); i++ {
					incompleteChunks[i] -= firstChunk
				}

				if offsetInSecondChunk == 0 {
					incompleteChunks = append(incompleteChunks[:0], incompleteChunks[reusableChunkIndex:]...)
				} else {
					// This should only happen rarely, especially with a
					// sufficiently large horizon.
					secondChunkRecomputedRegion := d[firstChunk:][:min+offsetInSecondChunk-1]
					var currentRecomputedHash uint64
					for _, b := range secondChunkRecomputedRegion[min-window : min] {
						currentRecomputedHash = (currentRecomputedHash << 1) + g[b]
					}
					incompleteChunks[0] = 0
					bestRecomputedHash := currentRecomputedHash
					recomputedChunkIndex := 1
					originalChunksCount := len(incompleteChunks)
					for i, b := range secondChunkRecomputedRegion[min:] {
						currentRecomputedHash = (currentRecomputedHash << 1) + g[b]
						if bestRecomputedHash < currentRecomputedHash {
							bestRecomputedHash = currentRecomputedHash
							recomputedChunk := i + 1
							if recomputedChunkIndex < reusableChunkIndex {
								incompleteChunks[recomputedChunkIndex] = recomputedChunk
								recomputedChunkIndex++
							} else {
								incompleteChunks = append(incompleteChunks, recomputedChunk)
							}
						}
					}
					if recomputedChunkIndex < reusableChunkIndex {
						incompleteChunks = append(incompleteChunks[:recomputedChunkIndex], incompleteChunks[reusableChunkIndex:]...)
					} else if len(incompleteChunks) > originalChunksCount {
						slices.Reverse(incompleteChunks[reusableChunkIndex:originalChunksCount])
						slices.Reverse(incompleteChunks[originalChunksCount:])
						slices.Reverse(incompleteChunks[reusableChunkIndex:])
					}
				}
				break
			}

			reusableChunkIndex++
			if reusableChunkIndex == len(incompleteChunks) {
				incompleteChunks = incompleteChunks[:1]
				break
			}
		}
	}

	f.completeChunks = completeChunks
	f.incompleteChunks = incompleteChunks
	f.currentHash = currentHash
	f.bestHash = bestHash
	return firstChunk, true, f.WindowDigest(d[firstChunk-window : firstChunk])
}

// Reset prepares the Chunker to split r from the start, reusing its buffers.
func (c *Chunker) Reset(r io.Reader) { c.core.Reset(r) }

// Next advances to the next chunk, returning false at end of input or on the
// first error.
func (c *Chunker) Next() bool { return c.core.Next() }

// Bytes returns the current chunk, valid until the next call to Next.
func (c *Chunker) Bytes() []byte { return c.core.Bytes() }

// ContentDefined reports whether the current chunk ended at a content-defined
// boundary (the repeated-maximum search picked it) rather than being forced at
// the end of the stream.
func (c *Chunker) ContentDefined() bool { return c.core.ContentDefined() }

// Sum returns the Gear fingerprint of the 64-byte window ending at the current
// chunk's cut. At a content-defined boundary it is the maximal value the
// repeated-maximum search selected. It is 0 only for a final chunk within 64
// bytes of the start of the stream.
func (c *Chunker) Sum() uint64 { return c.core.Sum() }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *Chunker) Offset() int { return c.core.Offset() }

// WindowSize returns 64: the Gear fingerprint is windowless, but its
// accumulator retains only the last 64 bytes.
func (c *Chunker) WindowSize() int { return c.core.WindowSize() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.core.Err() }
