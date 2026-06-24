// Package rollinghash/bozo64 is a 64-bit version of bozo32. Like bozo32,
// it is a wrong implementation of the rabinkarp checksum. In practice, it
// works very well and exhibits all the properties wanted from a rolling
// checksum, so it is kept despite not implementing the rabinkarp checksum
// as described in the original paper.

package bozo64

import (
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
)

// The size of the checksum.
const Size = 8

// Bozo64 is a digest which satisfies the rollinghash.Hash64 interface.
type Bozo64 struct {
	a     uint64
	aⁿ    uint64
	value uint64

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int
}

// Reset resets the Hash to its initial state.
func (d *Bozo64) Reset() {
	d.value = 0
	d.aⁿ = 1
	d.oldest = 0
	d.window = d.window[:0]
}

func NewFromInt(a uint64) *Bozo64 {
	return &Bozo64{
		a:      a,
		value:  0,
		aⁿ:     1,
		window: make([]byte, 0, rollinghash.DefaultWindowCap),
		oldest: 0,
	}
}

func New() *Bozo64 {
	return NewFromInt(4294967291) // largest prime fitting in 32 bits
}

// Size is 8 bytes
func (d *Bozo64) Size() int { return Size }

// BlockSize is 1 byte
func (d *Bozo64) BlockSize() int { return 1 }

// WriteWindow writes the contents of the current window to w.
func (d *Bozo64) WriteWindow(w io.Writer) (n int, err error) {
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
func (d *Bozo64) Write(data []byte) (int, error) {
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

	d.value = 0
	d.aⁿ = 1
	for _, c := range d.window {
		d.value *= d.a
		d.value += uint64(c)
		d.aⁿ *= d.a
	}
	return len(data), nil
}

// Sum64 returns the hash as a uint64
func (d *Bozo64) Sum64() uint64 {
	return d.value
}

// Sum returns the hash as byte slice
func (d *Bozo64) Sum(b []byte) []byte {
	v := d.Sum64()
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the entering byte. You
// MUST initialize a window with Write() before calling this method.
func (d *Bozo64) Roll(c byte) {
	// This check costs 10-15% performance. If we disable it, we crash
	// when the window is empty. If we enable it, we are always correct
	// (an empty window never changes no matter how much you roll it).
	//if len(d.window) == 0 {
	//	return
	//}
	// extract the entering/leaving bytes and update the circular buffer.
	enter := uint64(c)
	leave := uint64(d.window[d.oldest])
	d.window[d.oldest] = c
	l := len(d.window)
	d.oldest += 1
	if d.oldest >= l {
		d.oldest = 0
	}

	d.value = d.value*d.a + enter - leave*d.aⁿ
}

// Compile-time check that we implement the bulk fast path.
var _ rollinghash.BulkRoller = (*Bozo64)(nil)

// BulkRoll computes the rolling checksum of every window-sized slice of
// data in one pass and writes them to dst, which must have
// len(data)-window+1 elements: dst[i] is the checksum of data[i:i+window].
// It is equivalent to Write(data[:window]) followed by a Roll for each
// subsequent byte, recording Sum64 after each step, but it indexes the
// leaving byte directly (data[i]) instead of keeping a circular window and
// rolls two independent lanes so their multiplies overlap in the pipeline.
// BulkRoll does not modify the receiver; only d.a (the multiplier) is read.
func (d *Bozo64) BulkRoll(dst []uint64, data []byte, window int) {
	if window <= 0 || len(data) < window {
		return
	}
	a := d.a

	// aⁿ = a^window, the weight of the byte leaving the window.
	var aⁿ uint64 = 1
	for range window {
		aⁿ *= a
	}

	n := len(data) - window // highest output index; there are n+1 outputs.

	// Lane A owns dst[0:half], lane B owns dst[half:n+1]; the extra output
	// of an odd count goes to A.
	half := (n + 2) / 2

	// Lane A warmup: Horner over data[0:window].
	var vA uint64
	for j := range window {
		vA = vA*a + uint64(data[j])
	}
	dst[0] = vA

	if half > n {
		// Only one output (n == 0), or nothing left for a second lane.
		for ia := range n {
			vA = vA*a + uint64(data[ia+window]) - uint64(data[ia])*aⁿ
			dst[ia+1] = vA
		}
		return
	}

	// Lane B warmup: Horner over data[half:half+window] (in bounds because
	// half <= n implies half+window <= len(data)).
	var vB uint64
	for j := range window {
		vB = vB*a + uint64(data[half+j])
	}
	dst[half] = vB

	// Step both lanes in lockstep; vA and vB are independent locals so the
	// compiler keeps them in registers and the two multiplies pipeline.
	ia, ib := 0, half
	for ia < half-1 && ib < n {
		vA = vA*a + uint64(data[ia+window]) - uint64(data[ia])*aⁿ
		dst[ia+1] = vA
		vB = vB*a + uint64(data[ib+window]) - uint64(data[ib])*aⁿ
		dst[ib+1] = vB
		ia++
		ib++
	}
	// Finish whichever lane is longer (A, by at most one output).
	for ; ia < half-1; ia++ {
		vA = vA*a + uint64(data[ia+window]) - uint64(data[ia])*aⁿ
		dst[ia+1] = vA
	}
	for ; ib < n; ib++ {
		vB = vB*a + uint64(data[ib+window]) - uint64(data[ib])*aⁿ
		dst[ib+1] = vB
	}
}
