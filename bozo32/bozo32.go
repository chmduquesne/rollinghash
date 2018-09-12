// Package rollinghash/bozo32 is a wrong implementation of the rabinkarp
// checksum. In practice, it works very well and exhibits all the
// properties wanted from a rolling checksum, so after realising that this
// code did not implement the rabinkarp checksum as described in the
// original paper, it was renamed from rabinkarp32 to bozo32 and kept
// in this package.

package bozo32

import rollinghash "github.com/chmduquesne/rollinghash"

// The size of the checksum.
const Size = 4

// Bozo32 is a digest which satisfies the rollinghash.Hash32 interface.
type Bozo32 struct {
	a     uint32
	aⁿ    uint32
	value uint32

	window *rollinghash.RollingWindow
}

// Reset resets the Hash to its initial state.
func (d *Bozo32) Reset() {
	d.value = 0
	d.aⁿ = 1
	d.window.Reset()
}

func NewFromInt(a uint32) *Bozo32 {
	return &Bozo32{
		a:      a,
		value:  0,
		aⁿ:     1,
		window: rollinghash.NewRollingWindow(),
	}
}

func New() *Bozo32 {
	return NewFromInt(65521) // largest prime fitting in 16 bits
}

// Size is 4 bytes
func (d *Bozo32) Size() int { return Size }

// BlockSize is 1 byte
func (d *Bozo32) BlockSize() int { return 1 }

// Write appends data to the rolling window and updates the digest. It
// never returns an error.
func (d *Bozo32) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	d.window.Write(data)

	d.value = 0
	d.aⁿ = 1
	for _, c := range d.window.Bytes {
		d.value *= d.a
		d.value += uint32(c)
		d.aⁿ *= d.a
	}
	return len(data), nil
}

// Sum32 returns the hash as a uint32
func (d *Bozo32) Sum32() uint32 {
	return d.value
}

// Sum returns the hash as byte slice
func (d *Bozo32) Sum(b []byte) []byte {
	v := d.Sum32()
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the entering byte. You
// MUST initialize a window with Write() before calling this method.
func (d *Bozo32) Roll(b byte) {
	enter, leave := uint32(b), uint32(d.window.Roll(b))
	d.value = d.value*d.a + enter - leave*d.aⁿ
}
