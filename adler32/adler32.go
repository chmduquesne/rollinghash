// Package rollinghash/adler32 implements a rolling version of hash/adler32

package adler32

import (
	"hash"
	vanilla "hash/adler32"

	"github.com/chmduquesne/rollinghash"
)

const (
	Mod  = 65521
	Size = 4
)

// Adler32 is a digest which satisfies the rollinghash.Hash32 interface.
// It implements the adler32 algorithm https://en.wikipedia.org/wiki/Adler-32
type Adler32 struct {
	a, b uint32
	n    uint32

	window *rollinghash.RollingWindow

	vanilla hash.Hash32
}

// Reset resets the digest to its initial state.
func (d *Adler32) Reset() {
	d.a = 1
	d.b = 0
	d.n = 0
	d.window.Reset()
	d.vanilla.Reset()
}

// New returns a new Adler32 digest
func New() *Adler32 {
	return &Adler32{
		a:       1,
		b:       0,
		n:       0,
		window:  rollinghash.NewRollingWindow(),
		vanilla: vanilla.New(),
	}
}

// Size is 4 bytes
func (d *Adler32) Size() int { return Size }

// BlockSize is 1 byte
func (d *Adler32) BlockSize() int { return 1 }

// Write appends data to the rolling window and updates the digest.
func (d *Adler32) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	d.window.Write(data)

	// Piggy-back on the core implementation
	d.vanilla.Reset()
	d.vanilla.Write(d.window.Bytes)
	s := d.vanilla.Sum32()
	d.a, d.b = s&0xffff, s>>16
	d.n = uint32(len(d.window.Bytes)) % Mod
	return len(data), nil
}

// Sum32 returns the hash as a uint32
func (d *Adler32) Sum32() uint32 {
	return d.b<<16 | d.a
}

// Sum returns the hash as a byte slice
func (d *Adler32) Sum(b []byte) []byte {
	v := d.Sum32()
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the entering byte. You
// MUST initialize a window with Write() before calling this method.
func (d *Adler32) Roll(b byte) {
	// extract the entering/leaving bytes and update the circular buffer.
	enter, leave := uint32(b), uint32(d.window.Roll(b))

	// See http://stackoverflow.com/questions/40985080/why-does-my-rolling-adler32-checksum-not-work-in-go-modulo-arithmetic
	d.a = (d.a + Mod + enter - leave) % Mod
	d.b = (d.b + (d.n*leave/Mod+1)*Mod + d.a - (d.n * leave) - 1) % Mod
}
