package boxo

import (
	"io"
	"math"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/rabinkarp64"
)

// IpfsRabinPoly is the degree-53 irreducible polynomial boxo/chunker uses for
// Rabin fingerprinting (its chunk.IpfsRabinPoly).
var IpfsRabinPoly = rabinkarp64.Pol(17437180132763653)

// rabinWindow is the fingerprint window of github.com/whyrusleeping/chunker,
// the fork boxo wraps. Unlike restic/chunker's 64, it is 16.
const rabinWindow = 16

// Rabin splits content with Rabin fingerprints, matching boxo/chunker.Rabin.
type Rabin struct {
	ch     rollinghash.Chunker
	reader io.Reader
}

// NewRabin creates a Rabin splitter targeting the given average block size.
// Bounds are derived as boxo does: min = avg/3, max = avg + avg/2.
func NewRabin(r io.Reader, avgBlkSize uint64) *Rabin {
	min := avgBlkSize / 3
	max := avgBlkSize + (avgBlkSize / 2)
	return NewRabinMinMax(r, min, avgBlkSize, max)
}

// NewRabinMinMax creates a Rabin splitter with explicit min, average and max
// block sizes.
func NewRabinMinMax(r io.Reader, min, avg, max uint64) *Rabin {
	// whyrusleeping/chunker: sizeMask = (1 << uint(math.Log2(avg))) - 1.
	mask := uint64(1)<<uint(math.Log2(float64(avg))) - 1
	h := rabinkarp64.NewFromPol(IpfsRabinPoly)
	return &Rabin{
		ch:     rollinghash.NewChunker(r, h, rabinWindow, mask, rollinghash.WithBoundaries(int(min), int(max))),
		reader: r,
	}
}

// NextBytes returns the next chunk, or io.EOF once the stream is drained.
func (r *Rabin) NextBytes() ([]byte, error) {
	if !r.ch.Next() {
		if err := r.ch.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	b := r.ch.Bytes()
	return append([]byte(nil), b...), nil
}

// Reader returns the io.Reader associated with this Splitter.
func (r *Rabin) Reader() io.Reader { return r.reader }
