// Package rollinghash/adler32 implements a rolling version of hash/adler32

package adler32

import (
	"hash"
	vanilla "hash/adler32"
	"io"

	"github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/internal/window"
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

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int

	vanilla hash.Hash32
}

// Reset resets the digest to its initial state.
func (d *Adler32) Reset() {
	d.window = d.window[:0] // Reset the size but don't reallocate
	d.oldest = 0
	d.a = 1
	d.b = 0
	d.n = 0
	d.vanilla.Reset()
}

// New returns a new Adler32 digest
func New() *Adler32 {
	return &Adler32{
		a:       1,
		b:       0,
		n:       0,
		window:  make([]byte, 0, rollinghash.DefaultWindowCap),
		oldest:  0,
		vanilla: vanilla.New(),
	}
}

// Size is 4 bytes
func (d *Adler32) Size() int { return Size }

// BlockSize is 1 byte
func (d *Adler32) BlockSize() int { return 1 }

// WriteWindow writes the contents of the current window to w.
func (d *Adler32) WriteWindow(w io.Writer) (n int, err error) {
	return window.Write(w, d.window, d.oldest)
}

// Write appends data to the rolling window and updates the digest.
func (d *Adler32) Write(data []byte) (int, error) {
	l := len(data)
	if l == 0 {
		return 0, nil
	}
	// Re-arrange the window so that the leftmost element is at index 0
	if d.oldest != 0 {
		window.MoveLeft(d.window, d.oldest)
		d.oldest = 0
	}
	d.window = append(d.window, data...)

	// Piggy-back on the core implementation
	d.vanilla.Reset()
	d.vanilla.Write(d.window)
	s := d.vanilla.Sum32()
	d.a, d.b = s&0xffff, s>>16
	d.n = uint32(len(d.window)) % Mod
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
	// This check costs 10-15% performance. If we disable it, we crash
	// when the window is empty. If we enable it, we are always correct
	// (an empty window never changes no matter how much you roll it).
	//if len(d.window) == 0 {
	//	return
	//}
	// extract the entering/leaving bytes and update the circular buffer.
	enter := uint32(b)
	leave := uint32(d.window[d.oldest])
	d.window[d.oldest] = b
	d.oldest += 1
	if d.oldest >= len(d.window) {
		d.oldest = 0
	}

	d.a = (d.a + Mod + enter - leave) % Mod
	d.b = (d.b + d.a + Mod - 1 - (d.n*leave)%Mod) % Mod
}

// BulkRoll computes the rolling checksum of every window-sized slice of data
// in one pass and writes them to dst, which must have len(data)-window+1
// elements: dst[i] is the checksum of data[i:i+window] (the 32-bit value
// zero-extended into a uint64). It is equivalent to Write(data[:window])
// followed by a Roll for each subsequent byte, recording Sum32 after each
// step, but it indexes the leaving byte directly (data[i]) instead of keeping
// a circular window and rolls two independent lanes so their modular-arithmetic
// chains overlap in the pipeline. BulkRoll does not modify the receiver.
func (d *Adler32) BulkRoll(dst []uint64, data []byte, window int) {
	if window <= 0 || len(data) < window {
		return
	}
	wmod := uint32(window) % Mod // window length, as used in the b update

	n := len(data) - window // highest output index; there are n+1 outputs.

	// Lane A owns dst[0:half], lane B owns dst[half:n+1]; the extra output
	// of an odd count goes to A.
	half := (n + 2) / 2

	// Lane A warmup: accumulate adler over data[0:window].
	var aA, bA uint32 = 1, 0
	for j := range window {
		aA = (aA + uint32(data[j])) % Mod
		bA = (bA + aA) % Mod
	}
	dst[0] = uint64(bA<<16 | aA)

	if half > n {
		// Only one output (n == 0), or nothing left for a second lane.
		for ia := range n {
			leave, enter := uint32(data[ia]), uint32(data[ia+window])
			aA = (aA + Mod + enter - leave) % Mod
			bA = (bA + aA + Mod - 1 - (wmod*leave)%Mod) % Mod
			dst[ia+1] = uint64(bA<<16 | aA)
		}
		return
	}

	// Lane B warmup: accumulate adler over data[half:half+window].
	var aB, bB uint32 = 1, 0
	for j := range window {
		aB = (aB + uint32(data[half+j])) % Mod
		bB = (bB + aB) % Mod
	}
	dst[half] = uint64(bB<<16 | aB)

	// Step both lanes in lockstep; the two (a,b) pairs are independent locals
	// so the compiler keeps them in registers and their reductions pipeline.
	ia, ib := 0, half
	for ia < half-1 && ib < n {
		la, ea := uint32(data[ia]), uint32(data[ia+window])
		aA = (aA + Mod + ea - la) % Mod
		bA = (bA + aA + Mod - 1 - (wmod*la)%Mod) % Mod
		dst[ia+1] = uint64(bA<<16 | aA)

		lb, eb := uint32(data[ib]), uint32(data[ib+window])
		aB = (aB + Mod + eb - lb) % Mod
		bB = (bB + aB + Mod - 1 - (wmod*lb)%Mod) % Mod
		dst[ib+1] = uint64(bB<<16 | aB)

		ia++
		ib++
	}
	// Finish whichever lane is longer (A, by at most one output).
	for ; ia < half-1; ia++ {
		la, ea := uint32(data[ia]), uint32(data[ia+window])
		aA = (aA + Mod + ea - la) % Mod
		bA = (bA + aA + Mod - 1 - (wmod*la)%Mod) % Mod
		dst[ia+1] = uint64(bA<<16 | aA)
	}
	for ; ib < n; ib++ {
		lb, eb := uint32(data[ib]), uint32(data[ib+window])
		aB = (aB + Mod + eb - lb) % Mod
		bB = (bB + aB + Mod - 1 - (wmod*lb)%Mod) % Mod
		dst[ib+1] = uint64(bB<<16 | aB)
	}
}

// BulkBoundaries reports the window positions where the rolling checksum
// satisfies sum & mask == 0, fusing the test into the hashing loop (see
// the boundary fast path). It mirrors BulkRoll exactly, replacing each
// "dst[i] = uint64(b<<16 | a)" with the masked test on that value. It does not
// modify the receiver. (a and b are the lane hit buffers; the adler
// accumulators are aA/bA and aB/bB.)
func (d *Adler32) BulkBoundaries(a, b []int32, data []byte, window int, mask uint64) (na, nb int) {
	if window <= 0 || len(data) < window {
		return 0, 0
	}
	wmod := uint32(window) % Mod

	n := len(data) - window
	half := (n + 2) / 2

	var aA, bA uint32 = 1, 0
	for j := range window {
		aA = (aA + uint32(data[j])) % Mod
		bA = (bA + aA) % Mod
	}
	if uint64(bA<<16|aA)&mask == 0 {
		a[na] = 0
		na++
	}

	if half > n {
		for ia := range n {
			leave, enter := uint32(data[ia]), uint32(data[ia+window])
			aA = (aA + Mod + enter - leave) % Mod
			bA = (bA + aA + Mod - 1 - (wmod*leave)%Mod) % Mod
			if uint64(bA<<16|aA)&mask == 0 {
				a[na] = int32(ia + 1)
				na++
			}
		}
		return na, 0
	}

	var aB, bB uint32 = 1, 0
	for j := range window {
		aB = (aB + uint32(data[half+j])) % Mod
		bB = (bB + aB) % Mod
	}
	if uint64(bB<<16|aB)&mask == 0 {
		b[nb] = int32(half)
		nb++
	}

	ia, ib := 0, half
	for ia < half-1 && ib < n {
		la, ea := uint32(data[ia]), uint32(data[ia+window])
		aA = (aA + Mod + ea - la) % Mod
		bA = (bA + aA + Mod - 1 - (wmod*la)%Mod) % Mod
		if uint64(bA<<16|aA)&mask == 0 {
			a[na] = int32(ia + 1)
			na++
		}

		lb, eb := uint32(data[ib]), uint32(data[ib+window])
		aB = (aB + Mod + eb - lb) % Mod
		bB = (bB + aB + Mod - 1 - (wmod*lb)%Mod) % Mod
		if uint64(bB<<16|aB)&mask == 0 {
			b[nb] = int32(ib + 1)
			nb++
		}

		ia++
		ib++
	}
	for ; ia < half-1; ia++ {
		la, ea := uint32(data[ia]), uint32(data[ia+window])
		aA = (aA + Mod + ea - la) % Mod
		bA = (bA + aA + Mod - 1 - (wmod*la)%Mod) % Mod
		if uint64(bA<<16|aA)&mask == 0 {
			a[na] = int32(ia + 1)
			na++
		}
	}
	for ; ib < n; ib++ {
		lb, eb := uint32(data[ib]), uint32(data[ib+window])
		aB = (aB + Mod + eb - lb) % Mod
		bB = (bB + aB + Mod - 1 - (wmod*lb)%Mod) % Mod
		if uint64(bB<<16|aB)&mask == 0 {
			b[nb] = int32(ib + 1)
			nb++
		}
	}
	return na, nb
}
