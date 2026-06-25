// Package rollinghash/bozo32 is a wrong implementation of the rabinkarp
// checksum. In practice, it works very well and exhibits all the
// properties wanted from a rolling checksum, so after realising that this
// code did not implement the rabinkarp checksum as described in the
// original paper, it was renamed from rabinkarp32 to bozo32 and kept
// in this package.

package bozo32

import (
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
)

// The size of the checksum.
const Size = 4

// Bozo32 is a digest which satisfies the rollinghash.Hash32 interface.
type Bozo32 struct {
	a     uint32
	aⁿ    uint32
	value uint32

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int
}

// Reset resets the Hash to its initial state.
func (d *Bozo32) Reset() {
	d.value = 0
	d.aⁿ = 1
	d.oldest = 0
	d.window = d.window[:0]
}

func NewFromInt(a uint32) *Bozo32 {
	return &Bozo32{
		a:      a,
		value:  0,
		aⁿ:     1,
		window: make([]byte, 0, rollinghash.DefaultWindowCap),
		oldest: 0,
	}
}

func New() *Bozo32 {
	return NewFromInt(65521) // largest prime fitting in 16 bits
}

// Size is 4 bytes
func (d *Bozo32) Size() int { return Size }

// BlockSize is 1 byte
func (d *Bozo32) BlockSize() int { return 1 }

// WriteWindow writes the contents of the current window to w.
func (d *Bozo32) WriteWindow(w io.Writer) (n int, err error) {
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
func (d *Bozo32) Write(data []byte) (int, error) {
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
func (d *Bozo32) Roll(c byte) {
	// This check costs 10-15% performance. If we disable it, we crash
	// when the window is empty. If we enable it, we are always correct
	// (an empty window never changes no matter how much you roll it).
	//if len(d.window) == 0 {
	//	return
	//}
	// extract the entering/leaving bytes and update the circular buffer.
	enter := uint32(c)
	leave := uint32(d.window[d.oldest])
	d.window[d.oldest] = c
	l := len(d.window)
	d.oldest += 1
	if d.oldest >= l {
		d.oldest = 0
	}

	d.value = d.value*d.a + enter - leave*d.aⁿ
}

// BulkRoll computes the rolling checksum of every window-sized slice of
// data in one pass and writes them to dst, which must have
// len(data)-window+1 elements: dst[i] is the checksum of data[i:i+window]
// (the 32-bit value zero-extended into a uint64). It is equivalent to
// Write(data[:window]) followed by a Roll for each subsequent byte,
// recording Sum32 after each step, but it indexes the leaving byte directly
// (data[i]) instead of keeping a circular window and rolls two independent
// lanes so their multiplies overlap in the pipeline. BulkRoll does not
// modify the receiver; only d.a (the multiplier) is read.
func (d *Bozo32) BulkRoll(dst []uint64, data []byte, window int) {
	if window <= 0 || len(data) < window {
		return
	}
	a := d.a

	// aⁿ = a^window, the weight of the byte leaving the window.
	var aⁿ uint32 = 1
	for range window {
		aⁿ *= a
	}

	n := len(data) - window // highest output index; there are n+1 outputs.

	// Lane A owns dst[0:half], lane B owns dst[half:n+1]; the extra output
	// of an odd count goes to A.
	half := (n + 2) / 2

	// Lane A warmup: Horner over data[0:window].
	var vA uint32
	for j := range window {
		vA = vA*a + uint32(data[j])
	}
	dst[0] = uint64(vA)

	if half > n {
		// Only one output (n == 0), or nothing left for a second lane.
		for ia := range n {
			vA = vA*a + uint32(data[ia+window]) - uint32(data[ia])*aⁿ
			dst[ia+1] = uint64(vA)
		}
		return
	}

	// Lane B warmup: Horner over data[half:half+window] (in bounds because
	// half <= n implies half+window <= len(data)).
	var vB uint32
	for j := range window {
		vB = vB*a + uint32(data[half+j])
	}
	dst[half] = uint64(vB)

	// Step both lanes in lockstep; vA and vB are independent locals so the
	// compiler keeps them in registers and the two multiplies pipeline.
	ia, ib := 0, half
	for ia < half-1 && ib < n {
		vA = vA*a + uint32(data[ia+window]) - uint32(data[ia])*aⁿ
		dst[ia+1] = uint64(vA)
		vB = vB*a + uint32(data[ib+window]) - uint32(data[ib])*aⁿ
		dst[ib+1] = uint64(vB)
		ia++
		ib++
	}
	// Finish whichever lane is longer (A, by at most one output).
	for ; ia < half-1; ia++ {
		vA = vA*a + uint32(data[ia+window]) - uint32(data[ia])*aⁿ
		dst[ia+1] = uint64(vA)
	}
	for ; ib < n; ib++ {
		vB = vB*a + uint32(data[ib+window]) - uint32(data[ib])*aⁿ
		dst[ib+1] = uint64(vB)
	}
}

// BulkBoundaries reports the window positions where the rolling checksum
// satisfies sum & mask == 0, fusing the test into the hashing loop (see
// the boundary fast path). It mirrors BulkRoll exactly, replacing each
// "dst[i] = uint64(v)" with the masked test on the zero-extended value. It does
// not modify the receiver.
func (d *Bozo32) BulkBoundaries(a, b []int32, data []byte, window int, mask uint64) (na, nb int) {
	if window <= 0 || len(data) < window {
		return 0, 0
	}
	mul := d.a

	var aⁿ uint32 = 1
	for range window {
		aⁿ *= mul
	}

	n := len(data) - window
	half := (n + 2) / 2

	var vA uint32
	for j := range window {
		vA = vA*mul + uint32(data[j])
	}
	if uint64(vA)&mask == 0 {
		a[na] = 0
		na++
	}

	if half > n {
		for ia := range n {
			vA = vA*mul + uint32(data[ia+window]) - uint32(data[ia])*aⁿ
			if uint64(vA)&mask == 0 {
				a[na] = int32(ia + 1)
				na++
			}
		}
		return na, 0
	}

	var vB uint32
	for j := range window {
		vB = vB*mul + uint32(data[half+j])
	}
	if uint64(vB)&mask == 0 {
		b[nb] = int32(half)
		nb++
	}

	ia, ib := 0, half
	for ia < half-1 && ib < n {
		vA = vA*mul + uint32(data[ia+window]) - uint32(data[ia])*aⁿ
		if uint64(vA)&mask == 0 {
			a[na] = int32(ia + 1)
			na++
		}
		vB = vB*mul + uint32(data[ib+window]) - uint32(data[ib])*aⁿ
		if uint64(vB)&mask == 0 {
			b[nb] = int32(ib + 1)
			nb++
		}
		ia++
		ib++
	}
	for ; ia < half-1; ia++ {
		vA = vA*mul + uint32(data[ia+window]) - uint32(data[ia])*aⁿ
		if uint64(vA)&mask == 0 {
			a[na] = int32(ia + 1)
			na++
		}
	}
	for ; ib < n; ib++ {
		vB = vB*mul + uint32(data[ib+window]) - uint32(data[ib])*aⁿ
		if uint64(vB)&mask == 0 {
			b[nb] = int32(ib + 1)
			nb++
		}
	}
	return na, nb
}
