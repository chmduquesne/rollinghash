// Package rollinghash/adler32 implements a rolling version of hash/adler32

package adler32

import (
	"errors"

	rollinghash "gopkg.in/chmduquesne/rollinghash.v1"
)

const (
	mod = 65521
)

// The size of an Adler-32 checksum.
const Size = 4

// digest represents the partial evaluation of a checksum.
type digest struct {
	n     uint32
	a     uint32
	sum32 uint32
	ak    uint32

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int
}

// Reset resets the Hash to its initial state.
func (d *digest) Reset() {
	d.window = nil
	d.oldest = 0
}

func New(a, n uint32) rollinghash.Hash32 {
	return &digest{a: a, n: n, sum32: 0, window: nil, oldest: 0}
}

// Size returns the number of bytes Sum will return.
func (d *digest) Size() int { return Size }

// BlockSize returns the hash's underlying block size.
// The Write method must be able to accept any amount
// of data, but it may operate more efficiently if all
// writes are a multiple of the block size.
func (d *digest) BlockSize() int { return 1 }

// Write (via the embedded io.Writer interface) adds more data to the
// running hash. It never returns an error.
func (d *digest) Write(p []byte) (int, error) {
	// Copy the window
	d.window = make([]byte, len(p))
	copy(d.window, p)
	n := len(d.window)
	a := uint32(1)
	for i, _ := range d.window {
		d.sum32 += uint32(p[n-1-i]) * a
		a *= d.a
	}
	d.ak = a
	d.sum32 %= d.n
	return len(d.window), nil
}

func (d *digest) Sum32() uint32 {
	return d.sum32
}

func (d *digest) Sum(b []byte) []byte {
	v := d.Sum32()
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the leaving byte and the
// entering byte. See
// - http://www.samba.org/~tridge/phd_thesis.pdf (p. 55)
// - https://groups.google.com/forum/?fromgroups=#!topic/golang-nuts/ZiBcYH3Qw1g
// - https://github.com/josvazg/slicesync/blob/master/rollingadler32.go
func (d *digest) Roll(b byte) error {
	if len(d.window) == 0 {
		return errors.New(
			"the rolling window must be initialized with Write() first")
	}
	// extract the entering/leaving bytes and update the circular buffer.
	enter := uint32(b)
	leave := uint32(d.window[d.oldest])
	d.window[d.oldest] = b
	n := len(d.window)
	// d.oldest = (d.oldest + 1) % n // very slow
	// This is incredibly faster
	d.oldest += 1
	if d.oldest >= n {
		d.oldest = 0
	}

	d.sum32 = (d.sum32-leave*d.ak)*d.a + enter
	return nil
}
