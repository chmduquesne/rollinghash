// Package rollinghash/seahash64 implements the seahash checksum

package seahash64

import "github.com/chmduquesne/rollinghash"

const (
	Size  = 8
	seed1 = 0x16f11fe89b0d677c
	seed2 = 0xb480a793d8e6c86c
	seed3 = 0x6fe2e5aaf078ebc9
	seed4 = 0x14f994a4c5259381
	p     = 0x6eed0e9da4d94a4f
)

// Seahash64 is a digest which satisfies the rollinghash.Hash64 interface
type Seahash64 struct {
	a, b, c, d uint64

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int
}

// Reset resets the Hash to its initial state.
func (d *Seahash64) Reset() {
	d.window = d.window[:0]
	d.oldest = 0
}

// New returns a Seehash64 based on the default seeds
func New() *Seahash64 {
	return NewFromSeeds(seed1, seed2, seed3, seed4)
}

// NewFromSeeds returns a buzhash based on the provided table uint64 values.
func NewFromSeeds(seed1, seed2, seed3, seed4 uint64) *Seahash64 {
	return &Seahash64{
		a:      seed1,
		b:      seed2,
		c:      seed3,
		d:      seed4,
		window: make([]byte, 1, rollinghash.DefaultWindowCap),
		oldest: 0,
	}
}

func diffuse(x uint64) uint64 {
	x *= p
	x ^= (x >> 32) >> (x >> 60)
	x *= p
	return x
}

// Size is 8 bytes
func (d *Seahash64) Size() int { return Size }

// BlockSize is 1 byte
func (d *Seahash64) BlockSize() int { return 1 }

// Write (re)initializes the rolling window with the input byte slice and
// adds its data to the digest.
func (d *Seahash64) Write(data []byte) (int, error) {
	// Copy the window, avoiding allocations where possible
	l := len(data)
	if l == 0 {
		l = 1
	}
	if len(d.window) != l {
		if cap(d.window) >= l {
			d.window = d.window[:l]
		} else {
			d.window = make([]byte, l)
		}
	}
	copy(d.window, data)

	var block []byte
	for len(data) > 0 {
		if len(data) > 8 {
			block, data = data[:8], data[8:]
		} else {
			block, data = data, nil
		}
		u64 := uint64(0)
		for i := len(block) - 1; i >= 0; i-- {
			u64 <<= 8
			u64 |= uint64(block[i])
		}
		d.a, d.b, d.c, d.d = d.b, d.c, d.d, diffuse(d.a^u64)
	}
	return len(d.window), nil
}

// Sum64 returns the hash as a uint64
func (d *Seahash64) Sum64() uint64 {
	return diffuse(d.a ^ d.b ^ d.c ^ d.d ^ uint64(len(d.window)))
}

// Sum returns the hash as a byte slice
func (d *Seahash64) Sum(b []byte) []byte {
	v := d.Sum64()
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the entering byte. You
// MUST initialize a window with Write() before calling this method.
func (d *Seahash64) Roll(c byte) {
	// extract the entering/leaving bytes and update the circular buffer.

	d.window[d.oldest] = c
	l := len(d.window)
	d.oldest += 1
	if d.oldest >= l {
		d.oldest = 0
	}
}
