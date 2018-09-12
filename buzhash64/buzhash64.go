// Package rollinghash/buzhash implements buzhash as described by
// https://en.wikipedia.org/wiki/Rolling_hash#Cyclic_polynomial

package buzhash64

import (
	"math/rand"

	"github.com/chmduquesne/rollinghash"
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
	sum               uint64
	nRotate           uint
	nRotateComplement uint // redundant, but pre-computed to spare an operation
	bytehash          [256]uint64

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window *rollinghash.RollingWindow
}

// Reset resets the Hash to its initial state.
func (d *Buzhash64) Reset() {
	d.sum = 0
	d.window.Reset()
}

// GenerateHashes generates a list of hashes to use with buzhash
func GenerateHashes(seed int64) (res [256]uint64) {
	random := rand.New(rand.NewSource(seed))
	used := make(map[uint64]bool)
	for i, _ := range res {
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
		bytehash: b,
		window:   rollinghash.NewRollingWindow(),
	}
}

// Size is 8 bytes
func (d *Buzhash64) Size() int { return Size }

// BlockSize is 1 byte
func (d *Buzhash64) BlockSize() int { return 1 }

// Write appends data to the rolling window and updates the digest. It
// never returns an error.
func (d *Buzhash64) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	d.window.Write(data)

	d.sum = 0
	for _, c := range d.window.Bytes {
		d.sum = d.sum<<1 | d.sum>>63
		d.sum ^= d.bytehash[int(c)]
	}
	d.nRotate = uint(len(d.window.Bytes)) % 64
	d.nRotateComplement = 64 - d.nRotate
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
func (d *Buzhash64) Roll(b byte) {
	enter, leave := int(b), int(d.window.Roll(b))

	hn := d.bytehash[enter]
	h0 := d.bytehash[leave]

	d.sum = (d.sum<<1 | d.sum>>63) ^ (h0<<d.nRotate | h0>>d.nRotateComplement) ^ hn
}
