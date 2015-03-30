// Package rollinghash/adler32 implements a rolling version of hash/adler32

package adler32

import (
	"errors"
	"github.com/chmduquesne/rollinghash"
)

const (
	mod = 65521
)

// The size of an Adler-32 checksum b bytes.
const Size = 4

// digest represents the partial evaluation of a checksum.
type digest struct {
	// invariant: (a < mod && b < mod) || a <= b
	// invariant: a + b + 255 <= 0xffffffff
	a, b uint32

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int
}

// Reset resets the Hash to its initial state.
func (d *digest) Reset() { d.a, d.b = 1, 0 }

// New returns a new hash.Hash32 computing the rolling Adler-32 checksum.
// The window size will be determined by the length of the first write.
func New() rollinghash.RollingHash32 {
	d := new(digest)
	d.Reset()
	return d
}

// Size returns the number of bytes Sum will return.
func (d *digest) Size() int { return Size }

// BlockSize returns the hash's underlying block size.
// The Write method must be able to accept any amount
// of data, but it may operate more efficiently if all
// writes are a multiple of the block size.
func (d *digest) BlockSize() int { return 1 }

// Add data to the running checksum.
func update(a, b uint32, p []byte) (uint32, uint32) {
	for _, c := range p {
		a += uint32(c)
		b += a
		// invariant: a <= b
		if b > (0xffffffff-255)/2 {
			a %= mod
			b %= mod
			// invariant: a < mod && b < mod
		} else {
			// invariant: a + b + 255 <= 2 * b + 255 <= 0xffffffff
		}
	}
	return a, b
}

// finish returns the 32-bit checksum corresponding to a, b.
func finish(a, b uint32) uint32 {
	if b >= mod {
		a %= mod
		b %= mod
	}
	return b<<16 | a
}

// Write (via the embedded io.Writer interface) adds more data to the
// running hash. It never returns an error.
func (d *digest) Write(p []byte) (nn int, err error) {
	d.window = make([]byte, len(p))
	copy(d.window, p)
	d.a, d.b = update(d.a, d.b, d.window)
	return len(d.window), nil
}

func (d *digest) Sum32() uint32 { return finish(d.a, d.b) }

func (d *digest) Sum(b []byte) []byte {
	s := d.Sum32()
	b = append(b, byte(s>>24))
	b = append(b, byte(s>>16))
	b = append(b, byte(s>>8))
	b = append(b, byte(s))
	return b
}

// See http://www.samba.org/~tridge/phd_thesis.pdf (p. 55)
// See https://groups.google.com/forum/?fromgroups=#!topic/golang-nuts/ZiBcYH3Qw1g
// See https://github.com/josvazg/slicesync/blob/master/rollingadler32.go
func roll(a, b uint32, window, oldest, newest uint32) (aa, bb uint32) {
	a += newest - oldest
	b += a - (window * oldest) - 1
	// invariant: a <= b
	if b > (0xffffffff-255)/2 {
		a %= mod
		b %= mod
		// invariant: a < mod && b < mod
	} else {
		// invariant: a + b + 255 <= 2 * b + 255 <= 0xffffffff
	}
	return a, b
}

// Roll updates the checksum of the window from the leaving byte and the
// entering byte
func (d *digest) Roll(b byte) error {
	if len(d.window) == 0 {
		return errors.New(
			"The window must be initialized with Write() first.")
	}
	newbyte := b
	oldbyte := d.window[d.oldest]
	d.window[d.oldest] = b
	d.oldest = (d.oldest + 1) % len(d.window)
	d.a, d.b = roll(d.a, d.b, uint32(len(d.window)), uint32(oldbyte), uint32(newbyte))
	return nil
}
