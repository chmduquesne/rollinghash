// Package rollinghash/buzhash implements buzhash as described by
// https://en.wikipedia.org/wiki/Rolling_hash#Cyclic_polynomial
//
// CAVEAT: avoid window lengths that are a multiple of 64 (the word size).
// buzhash rolls the sum by rotating a 64-bit word one bit per byte, so
// after 64 bytes the rotation wraps. A run of >=window identical bytes
// (very common in binary data: zero padding, 0xff flash padding,
// alignment) then collapses the hash to a single degenerate value
// (all-ones for odd multiples of 64, zero for even multiples), losing all
// entropy. With a 64-byte window over typical executables this makes the
// hash equal 0xffffffffffffffff about 1% of the time. Any window length
// that is not a multiple of 64 avoids this. This is inherent to the cyclic
// polynomial construction and cannot be fixed by changing the byte table.

package buzhash64

import (
	"io"
	"math/bits"
	"math/rand"

	"github.com/chmduquesne/rollinghash/v4"
)

var defaultHashes [256]uint64

func init() {
	defaultHashes = GenerateHashes(1)
}

// The size of the checksum.
const Size = 8

// Buzhash64 is a digest which satisfies the rollinghash.Hash64 interface.
// It implements the cyclic polynomial algorithm
// https://en.wikipedia.org/wiki/Rolling_hash#Cyclic_polynomial
type Buzhash64 struct {
	sum     uint64
	nRotate uint

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window   []byte
	oldest   int
	bytehash [256]uint64
}

// Reset resets the Hash to its initial state.
func (d *Buzhash64) Reset() {
	d.window = d.window[:0]
	d.oldest = 0
	d.sum = 0
}

// GenerateHashes generates a list of hashes to use with buzhash
func GenerateHashes(seed int64) (res [256]uint64) {
	random := rand.New(rand.NewSource(seed))
	used := make(map[uint64]bool)
	for i := range res {
		x := uint64(random.Int63())
		for used[x] {
			x = uint64(random.Int63())
		}
		used[x] = true
		res[i] = x
	}
	return res
}

// New returns a buzhash based on a list of hashes provided by a call to
// GenerateHashes, seeded with the default value 1.
func New() *Buzhash64 {
	return NewFromUint64Array(defaultHashes)
}

// NewFromUint64Array returns a buzhash based on the provided table uint64 values.
func NewFromUint64Array(b [256]uint64) *Buzhash64 {
	return &Buzhash64{
		sum:      0,
		window:   make([]byte, 0, rollinghash.DefaultWindowCap),
		oldest:   0,
		bytehash: b,
	}
}

// Size is 8 bytes
func (d *Buzhash64) Size() int { return Size }

// BlockSize is 1 byte
func (d *Buzhash64) BlockSize() int { return 1 }

// WriteWindow writes the contents of the current window to w.
func (d *Buzhash64) WriteWindow(w io.Writer) (n int, err error) {
	// Copy the older bytes.
	if d.oldest < len(d.window) {
		n, err = w.Write(d.window[d.oldest:])
	}
	// Then the newer bytes.
	if err == nil && d.oldest > 0 {
		var n2 int
		n2, err = w.Write(d.window[:d.oldest])
		n += n2
	}
	return
}

// Write appends data to the rolling window and updates the digest. It
// never returns an error.
func (d *Buzhash64) Write(data []byte) (int, error) {
	l := len(data)
	if l == 0 {
		return 0, nil
	}
	// Re-arrange the window so that the leftmost element is at index 0
	n := len(d.window)
	if d.oldest != 0 {
		tmp := make([]byte, d.oldest)
		copy(tmp, d.window[:d.oldest])
		copy(d.window, d.window[d.oldest:])
		copy(d.window[n-d.oldest:], tmp)
		d.oldest = 0
	}
	d.window = append(d.window, data...)

	d.sum = 0
	for _, c := range d.window {
		d.sum = bits.RotateLeft64(d.sum, 1)
		d.sum ^= d.bytehash[int(c)]
	}
	d.nRotate = uint(len(d.window)) % 64
	return len(data), nil
}

// Sum64 returns the hash as a uint64
func (d *Buzhash64) Sum64() uint64 {
	return d.sum
}

// Sum returns the hash as a byte slice
func (d *Buzhash64) Sum(b []byte) []byte {
	v := d.Sum64()
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the entering byte. You
// MUST initialize a window with Write() before calling this method.
func (d *Buzhash64) Roll(c byte) {
	// This check costs 10-15% performance. If we disable it, we crash
	// when the window is empty. If we enable it, we are always correct
	// (an empty window never changes no matter how much you roll it).
	//if len(d.window) == 0 {
	//	return
	//}

	// extract the entering/leaving bytes and update the circular buffer.
	hn := d.bytehash[int(c)]
	h0 := d.bytehash[int(d.window[d.oldest])]

	d.window[d.oldest] = c
	l := len(d.window)
	d.oldest += 1
	if d.oldest >= l {
		d.oldest = 0
	}

	d.sum = bits.RotateLeft64(d.sum, 1) ^ bits.RotateLeft64(h0, int(d.nRotate)) ^ hn
}

// Compile-time check that we implement the bulk fast path.
var _ rollinghash.BulkRoller = (*Buzhash64)(nil)

// BulkRoll computes the rolling checksum of every window-sized slice of data
// in one pass and writes them to dst, which must have len(data)-window+1
// elements: dst[i] is the checksum of data[i:i+window]. It is equivalent to
// Write(data[:window]) followed by a Roll for each subsequent byte, recording
// Sum64 after each step, but it indexes the leaving byte directly (data[i])
// instead of keeping a circular window and rolls two independent lanes so
// their rotate/XOR chains overlap in the pipeline. BulkRoll does not modify
// the receiver; only d.bytehash is read.
func (d *Buzhash64) BulkRoll(dst []uint64, data []byte, window int) {
	if window <= 0 || len(data) < window {
		return
	}
	bh := &d.bytehash
	nRotate := window % 64 // rotation applied to the leaving byte's hash

	n := len(data) - window // highest output index; there are n+1 outputs.

	// Lane A owns dst[0:half], lane B owns dst[half:n+1]; the extra output
	// of an odd count goes to A.
	half := (n + 2) / 2

	// Lane A warmup over data[0:window].
	var vA uint64
	for j := range window {
		vA = bits.RotateLeft64(vA, 1) ^ bh[data[j]]
	}
	dst[0] = vA

	if half > n {
		// Only one output (n == 0), or nothing left for a second lane.
		for ia := range n {
			vA = bits.RotateLeft64(vA, 1) ^ bits.RotateLeft64(bh[data[ia]], nRotate) ^ bh[data[ia+window]]
			dst[ia+1] = vA
		}
		return
	}

	// Lane B warmup over data[half:half+window].
	var vB uint64
	for j := range window {
		vB = bits.RotateLeft64(vB, 1) ^ bh[data[half+j]]
	}
	dst[half] = vB

	// Step both lanes in lockstep; vA and vB are independent locals so the
	// compiler keeps them in registers and the two rotate/XOR chains pipeline.
	ia, ib := 0, half
	for ia < half-1 && ib < n {
		vA = bits.RotateLeft64(vA, 1) ^ bits.RotateLeft64(bh[data[ia]], nRotate) ^ bh[data[ia+window]]
		dst[ia+1] = vA
		vB = bits.RotateLeft64(vB, 1) ^ bits.RotateLeft64(bh[data[ib]], nRotate) ^ bh[data[ib+window]]
		dst[ib+1] = vB
		ia++
		ib++
	}
	// Finish whichever lane is longer (A, by at most one output).
	for ; ia < half-1; ia++ {
		vA = bits.RotateLeft64(vA, 1) ^ bits.RotateLeft64(bh[data[ia]], nRotate) ^ bh[data[ia+window]]
		dst[ia+1] = vA
	}
	for ; ib < n; ib++ {
		vB = bits.RotateLeft64(vB, 1) ^ bits.RotateLeft64(bh[data[ib]], nRotate) ^ bh[data[ib+window]]
		dst[ib+1] = vB
	}
}
