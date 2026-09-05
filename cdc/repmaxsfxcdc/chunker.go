// Package repmaxsfxcdc splits a stream into content-defined chunks using
// RepMaxSfxCDC ("repeated maximum, suffix"), from buildbarn/go-cdc.
//
// Like RepMaxCDC it produces chunks strictly in [minSize, 2*minSize) by
// repeatedly picking an extremum within a read-ahead horizon and restricting
// the horizon when the pick would overshoot. The difference is what "extremum"
// means: RepMaxSfxCDC uses no rolling hash. It cuts before the position whose
// following minSize bytes form the lexicographically largest string, comparing
// those minSize-byte strings directly. Not depending on a fixed hash window
// makes cut-point selection more stable.
//
// Parameters are minSize and a horizon size (as in repmaxcdc): the horizon only
// controls chunking quality and can be raised freely. The maximum chunk size is
// fixed at 2*minSize (exclusive); the trailing chunk of a stream may be
// shorter.
//
// Boundaries match buildbarn/go-cdc's NewRepMaxSfxContentDefinedChunker (with
// the identity substitution box) and NewSimpleRepMaxSfxContentDefinedChunker for
// the same minSize and horizon.
//
// New returns a pull-based *Chunker (rollinghash.Chunker) over an io.Reader;
// NewChunkWriter returns the push-based *ChunkWriter (rollinghash.ChunkWriter),
// fed via Write/Close.
package repmaxsfxcdc

import (
	"io"
	"math/bits"
	"slices"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/cutcore"
)

// A Chunker splits an io.Reader into content-defined chunks using RepMaxSfxCDC.
//
//	c := repmaxsfxcdc.New(r, minSize, horizon)
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
type Option func(*sfxCut)

// WithBuffer supplies the working buffer, adopted when its capacity is large
// enough (roughly 2*(2*minSize+horizon)). Use it to reuse one allocation across
// many streams.
func WithBuffer(buf []byte) Option {
	return func(f *sfxCut) { f.buf = buf }
}

// identityBox is the substitution box used when WithSubstitutionBox isn't
// given: comparisons see the raw bytes.
var identityBox = func() (t [256]byte) {
	for i := range t {
		t[i] = byte(i)
	}
	return
}()

// WithSubstitutionBox remaps every byte through box before comparing the
// candidate strings (the emitted bytes are unchanged). buildbarn/go-cdc uses a
// bijective S-box here to break up linearity in sorted input, which otherwise
// makes every record start a candidate cut. Pass a permutation of 0..255; the
// identity box is equivalent to not setting the option.
func WithSubstitutionBox(box [256]byte) Option {
	return func(f *sfxCut) { f.sbox = box }
}

// New returns a Chunker over r. Chunk lengths are kept in [minSize, 2*minSize);
// horizon is the read-ahead within which the repeated-maximum search always
// finds the optimal cut (0 gives uniform minSize chunks). New panics if
// minSize < 2 or horizon < 0.
func New(r io.Reader, minSize, horizon int, opts ...Option) *Chunker {
	f := newCut(minSize, horizon, opts)
	return &Chunker{core: cutcore.New(r, f, f.buf)}
}

func newCut(minSize, horizon int, opts []Option) *sfxCut {
	if minSize < 2 {
		panic("repmaxsfxcdc: minSize must be >= 2")
	}
	if horizon < 0 {
		panic("repmaxsfxcdc: horizon must be >= 0")
	}
	f := &sfxCut{min: minSize, peek: 2*minSize + horizon, sbox: identityBox}
	for _, opt := range opts {
		opt(f)
	}
	f.completeChunks = make([]int, 0, f.peek/f.min+1)
	// oldChunks rarely grows past a handful of entries in practice (matching
	// buildbarn's own sizing); it can in principle grow with the horizon on
	// pathological input, in which case it just reallocates.
	f.oldChunks = make([]sfxCand, 0, 64)
	f.currentChunk = 1
	return f
}

// sfxCand is a run of equidistant candidate cutting points, kept on sfxCut's
// stack: for input like "...bababababac...", every "b" and "c" is a candidate,
// with "c" the most preferable; last holds c's offset and period holds 2 (the
// spacing), so the whole run is represented in O(1) instead of one entry per
// candidate.
type sfxCand struct {
	last   int
	period int
}

// sfxCut is the RepMaxSfxCDC cutpoint finder: a port of buildbarn/go-cdc's
// optimized repMaxSfxChunkReader.ReadNextChunk. Like repmaxcdc, it keeps its
// search state (the candidate stack, and how far the current best candidate's
// match against the input has progressed) across Cut calls, so each input byte
// is compared exactly once, and long runs of near-identical bytes collapse to
// O(1) via the period tracking above instead of driving repeated full-horizon
// rescans (the naive port of simpleRepMaxSfxChunkReader's failure mode, and
// why it was up to 3.5x slower than buildbarn's real implementation).
type sfxCut struct {
	min  int
	peek int // 2*min + horizon, the read-ahead buildbarn peeks per chunk
	sbox [256]byte
	buf  []byte

	// completeChunks holds, in reverse order (next chunk to emit is the last
	// element), the lengths of chunks no more data can influence.
	completeChunks []int
	// oldChunks holds candidate runs older than firstBest, oldest first.
	oldChunks []sfxCand
	// firstBest is the best candidate run found so far in this scan.
	firstBest sfxCand
	// lastBest is the last (most recent) occurrence of firstBest's data;
	// tracked separately from firstBest.last (its first occurrence, which is
	// what actually gets cut at) so periodicity can be computed as new
	// occurrences are found.
	lastBest int
	// currentChunk is the offset being tested as a candidate.
	currentChunk int
	// matchLength is how many bytes past currentChunk match those past
	// lastBest.
	matchLength int
}

func (f *sfxCut) MaxSize() int { return f.peek }

// Window: RepMaxSfxCDC has no rolling window; the strings it compares start at
// the candidate cut and reach forward into the next chunk. One byte is reported
// so Sum at a forced or final cut is well defined.
func (f *sfxCut) Window() int { return 1 }

// WindowDigest returns the value of the last byte of b, the Sum at a forced or
// final cut.
func (f *sfxCut) WindowDigest(b []byte) uint64 {
	if len(b) == 0 {
		return 0
	}
	return uint64(b[len(b)-1])
}

// ResetCut clears the carried-over search state so a Reset'd Chunker starts
// identically to a fresh one.
func (f *sfxCut) ResetCut() {
	f.completeChunks = f.completeChunks[:0]
	f.oldChunks = f.oldChunks[:0]
	f.firstBest = sfxCand{}
	f.lastBest = 0
	f.currentChunk = 1
	f.matchLength = 0
}

func (f *sfxCut) Cut(d []byte, eof bool) (int, bool, uint64) {
	min := f.min

	// Fast path: an earlier horizon scan already produced more than one
	// stable boundary. Hand them out one at a time with no further scanning.
	if n := len(f.completeChunks); n > 0 {
		size := f.completeChunks[n-1]
		f.completeChunks = f.completeChunks[:n-1]
		return size, true, uint64(d[size])
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
		f.oldChunks = f.oldChunks[:0]
		f.firstBest = sfxCand{}
		f.lastBest = 0
		f.currentChunk = 1
		f.matchLength = 0
		return len(d), false, 0
	}

	sbox := &f.sbox
	completeChunks := f.completeChunks[:0]
	uncompletedRegion := d[min:]
	oldChunks := f.oldChunks
	firstBest := f.firstBest
	lastBest := f.lastBest
	currentChunk := f.currentChunk
	matchLength := f.matchLength

	if matchLength != 0 {
		goto MatchSlow
	}

	// Optimize the case where we haven't yet matched any leading bytes, and
	// the region to scan is still sufficiently large: compare eight bytes at
	// once, similar to repmaxcdc's unrolled Gear scan.
MatchFirstBytes:
	if easyRegion := uncompletedRegion[currentChunk:min2(len(uncompletedRegion), firstBest.last+min)]; len(easyRegion) >= 7+8 {
		bestFirstBytes := uint64(sbox[uncompletedRegion[lastBest]])<<56 |
			uint64(sbox[uncompletedRegion[lastBest+1]])<<48 |
			uint64(sbox[uncompletedRegion[lastBest+2]])<<40 |
			uint64(sbox[uncompletedRegion[lastBest+3]])<<32 |
			uint64(sbox[uncompletedRegion[lastBest+4]])<<24 |
			uint64(sbox[uncompletedRegion[lastBest+5]])<<16 |
			uint64(sbox[uncompletedRegion[lastBest+6]])<<8 |
			uint64(sbox[uncompletedRegion[lastBest+7]])
		currentFirstBytes := uint64(sbox[easyRegion[0]])<<48 |
			uint64(sbox[easyRegion[1]])<<40 |
			uint64(sbox[easyRegion[2]])<<32 |
			uint64(sbox[easyRegion[3]])<<24 |
			uint64(sbox[easyRegion[4]])<<16 |
			uint64(sbox[easyRegion[5]])<<8 |
			uint64(sbox[easyRegion[6]])
		i := 7
		for ; i+8 <= len(easyRegion); i += 8 {
			b := easyRegion[i : i+8 : i+8]
			newFirstBytes := uint64(sbox[b[0]])
			mergedFirstBytes := (currentFirstBytes << 8) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 7
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[1]])
			mergedFirstBytes = (currentFirstBytes << 16) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 6
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[2]])
			mergedFirstBytes = (currentFirstBytes << 24) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 5
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[3]])
			mergedFirstBytes = (currentFirstBytes << 32) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 4
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[4]])
			mergedFirstBytes = (currentFirstBytes << 40) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 3
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[5]])
			mergedFirstBytes = (currentFirstBytes << 48) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 2
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[6]])
			mergedFirstBytes = (currentFirstBytes << 56) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 1
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[7]])
			mergedFirstBytes = newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			currentFirstBytes = mergedFirstBytes
		}
		currentChunk += i - 7
		goto MatchSlow
	}

MatchSlow:
	for currentChunk+matchLength < len(uncompletedRegion) {
		if ca, cb := sbox[uncompletedRegion[lastBest+matchLength]], sbox[uncompletedRegion[currentChunk+matchLength]]; ca > cb {
			// Current candidate is worse than what has already been
			// observed. Reset and match from the beginning.
			currentChunk += matchLength + 1
			if currentChunk >= firstBest.last+min {
				goto CompleteChunks
			}
			matchLength = 0
			goto MatchFirstBytes
		} else if ca < cb {
			// Current candidate is better than what has already been
			// observed. Store its offset.
			if distance := currentChunk - firstBest.last; firstBest.period == distance {
				firstBest.last = currentChunk
			} else {
				oldChunks = append(oldChunks, firstBest)
				firstBest = sfxCand{last: currentChunk, period: distance}
			}

			if period := currentChunk - lastBest; matchLength >= period {
				// The best potential cutting point is followed by repeated
				// data: register multiple potential cutting points.
				oldMatchLength := matchLength
				matchLength %= period
				currentChunk += oldMatchLength - matchLength
				if matchLength == 0 {
					lastBest = currentChunk
					currentChunk++
				} else {
					lastBest = currentChunk - period
				}
				if firstBest.last != lastBest {
					if firstBest.period == period {
						firstBest.last = lastBest
					} else {
						oldChunks = append(oldChunks, firstBest)
						firstBest = sfxCand{last: lastBest, period: period}
					}
				}
				if matchLength == 0 {
					goto MatchFirstBytes
				}
			} else {
				lastBest = currentChunk
				currentChunk++
				matchLength = 0
				goto MatchFirstBytes
			}
		} else {
			// Best candidate and current candidate share the same prefix.
			// Continue matching the next byte.
			matchLength++
			if matchLength == min {
				// Only compare up to min bytes of data, so as not to be
				// influenced by data residing beyond the resulting chunk. The
				// best and current candidate are equally good here; since the
				// algorithm prefers the first occurrence, the current
				// candidate isn't eligible, but is still respected for
				// periodicity's sake.
				period := currentChunk - lastBest
				currentChunk += period
				if currentChunk >= firstBest.last+min {
					goto CompleteChunks
				}
				lastBest += period
				matchLength -= period
			}
		}
		continue

	CompleteChunks:
		// None of the cutting points obtained thus far can be further
		// influenced by data that follows: complete all chunks up to this
		// point, in a reverse pass selecting ones at least min apart.
		uncompletedRegion = uncompletedRegion[min+firstBest.last:]
		previousCompleteChunksCount := len(completeChunks)
		for i := len(oldChunks) - 1; firstBest.last >= min; i-- {
			previousChunk := oldChunks[i]
			for {
				maxNextChunk := firstBest.last - min
				if previousChunk.last > maxNextChunk {
					break
				}
				chunk := previousChunk.last + (maxNextChunk-previousChunk.last)/firstBest.period*firstBest.period
				completeChunks = append(completeChunks, firstBest.last-chunk)
				firstBest.last = chunk
			}
			firstBest.period = previousChunk.period
		}
		completeChunks = append(completeChunks, min+firstBest.last)
		slices.Reverse(completeChunks[previousCompleteChunksCount:])

		// Reinitialize to compute chunks following the ones just completed.
		oldChunks = oldChunks[:0]
		firstBest = sfxCand{}
		lastBest = 0
		currentChunk = 1
		matchLength = 0
		if len(uncompletedRegion) <= 1 {
			break
		}
		goto MatchFirstBytes
	}

	// Processed the full horizon. Determine which chunk to emit now.
	var firstChunk int
	if len(completeChunks) > 0 {
		slices.Reverse(completeChunks)
		firstChunk = completeChunks[len(completeChunks)-1]
		completeChunks = completeChunks[:len(completeChunks)-1]
	} else {
		// The scan didn't naturally complete anything - either we hit end of
		// stream, or the horizon wasn't large enough. Pick a cutting point
		// that still respects the maximum chunk size while remaining the most
		// preferable one available. First remove cutting points discovered in
		// the minSize peeked beyond the horizon (to stay consistent with the
		// simple algorithm), then walk back to a legal chunk length.
		for len(oldChunks) > 0 && oldChunks[len(oldChunks)-1].last >= len(d)-2*min {
			firstBest = oldChunks[len(oldChunks)-1]
			oldChunks = oldChunks[:len(oldChunks)-1]
		}
		if firstBest.last > len(d)-2*min {
			firstBest.last -= (firstBest.last - (len(d) - 2*min) + (firstBest.period - 1)) / firstBest.period * firstBest.period
		}
		for i := len(oldChunks) - 1; firstBest.last >= min; i-- {
			previousChunk := oldChunks[i]
			for {
				maxNextChunk := firstBest.last - min
				if previousChunk.last > maxNextChunk {
					break
				}
				chunk := previousChunk.last + (maxNextChunk-previousChunk.last)/firstBest.period*firstBest.period
				firstBest.last = chunk
			}
			firstBest.period = previousChunk.period
		}

		// RepMaxCDC preserves cutting points across calls in this fallback,
		// only recomputing a small region; here we simply reinitialize, as
		// buildbarn's own optimized implementation does (this should only
		// happen rarely given a sufficiently large horizon).
		firstChunk = min + firstBest.last
		oldChunks = oldChunks[:0]
		firstBest = sfxCand{}
		lastBest = 0
		currentChunk = 1
		matchLength = 0
	}

	f.completeChunks = completeChunks
	f.oldChunks = oldChunks
	f.firstBest = firstBest
	f.lastBest = lastBest
	f.currentChunk = currentChunk
	f.matchLength = matchLength
	return firstChunk, true, uint64(d[firstChunk])
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
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

// Sum returns the first byte of the lexicographically-maximal minSize-byte
// string the cut was chosen for (i.e. the first byte of the next chunk). At a
// forced cut it is the final byte of the chunk instead. RepMaxSfxCDC uses no
// rolling hash, and this single byte carries less information than the
// Gear-based chunkers' Sum.
func (c *Chunker) Sum() uint64 { return c.core.Sum() }

// Offset returns the start byte offset of the current chunk in the stream.
func (c *Chunker) Offset() int { return c.core.Offset() }

// WindowSize returns 1: RepMaxSfxCDC has no rolling window (its comparison
// strings reach forward from the cut); one byte is reported for a well-defined
// forced-cut Sum.
func (c *Chunker) WindowSize() int { return c.core.WindowSize() }

// Err returns the first non-EOF error encountered by Next, if any.
func (c *Chunker) Err() error { return c.core.Err() }
