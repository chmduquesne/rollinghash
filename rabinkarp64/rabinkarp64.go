// Copyright (c) 2014, Alexander Neumann <alexander@bumpern.de>
// Copyright (c) 2017, Christophe-Marie Duquesne <chmd@chmd.fr>
//
// This file was adapted from restic https://github.com/restic/chunker
//
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package rabinkarp64

import (
	"io"
	"sync"

	"github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/internal/window"
)

const Size = 8

type tables struct {
	out [256]Pol
	mod [256]Pol
}

// tables are cacheable for a given pol and windowsize
type index struct {
	pol        Pol
	windowsize int
}

type RabinKarp64 struct {
	pol      Pol
	tables   *tables
	polShift uint
	value    Pol

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int
}

// cache precomputed tables, these are read-only anyway
var cache struct {
	// For a given polynom and a given window size, we get a table
	entries map[index]*tables
	sync.Mutex
}

func init() {
	cache.entries = make(map[index]*tables)
}

// tablesFor returns the (cached) reduction tables for a given polynomial and
// window size, building and caching them on a miss. The tables are read-only.
func tablesFor(pol Pol, windowsize int) *tables {
	idx := index{pol, windowsize}

	cache.Lock()
	t, ok := cache.entries[idx]
	cache.Unlock()
	if ok {
		return t
	}

	t = buildTables(pol, windowsize)
	cache.Lock()
	cache.entries[idx] = t
	cache.Unlock()
	return t
}

func (d *RabinKarp64) updateTables() {
	d.tables = tablesFor(d.pol, len(d.window))
}

func buildTables(pol Pol, windowsize int) (t *tables) {
	t = &tables{}
	// calculate table for sliding out bytes. The byte to slide out is used as
	// the index for the table, the value contains the following:
	// out_table[b] = Hash(b || 0 ||        ...        || 0)
	//                          \ windowsize-1 zero bytes /
	// To slide out byte b_0 for window size w with known hash
	// H := H(b_0 || ... || b_w), it is sufficient to add out_table[b_0]:
	//    H(b_0 || ... || b_w) + H(b_0 || 0 || ... || 0)
	//  = H(b_0 + b_0 || b_1 + 0 || ... || b_w + 0)
	//  = H(    0     || b_1 || ...     || b_w)
	//
	// Afterwards a new byte can be shifted in.
	for b := range 256 {
		var h Pol
		h <<= 8
		h |= Pol(b)
		h = h.Mod(pol)
		for range windowsize - 1 {
			h <<= 8
			h |= Pol(0)
			h = h.Mod(pol)
		}
		t.out[b] = h
	}

	// calculate table for reduction mod Polynomial
	k := pol.Deg()
	for b := range 256 {
		// mod_table[b] = A | B, where A = (b(x) * x^k mod pol) and  B = b(x) * x^k
		//
		// The 8 bits above deg(Polynomial) determine what happens next and so
		// these bits are used as a lookup to this table. The value is split in
		// two parts: Part A contains the result of the modulus operation, part
		// B is used to cancel out the 8 top bits so that one XOR operation is
		// enough to reduce modulo Polynomial
		t.mod[b] = Pol(uint64(b)<<uint(k)).Mod(pol) | (Pol(b) << uint(k))
	}

	return t
}

// NewFromPol returns a RabinKarp64 digest from a polynomial over GF(2).
// It is assumed that the input polynomial is irreducible. You can obtain
// such a polynomial using the RandomPolynomial function.
func NewFromPol(p Pol) *RabinKarp64 {
	res := &RabinKarp64{
		pol:      p,
		tables:   nil,
		polShift: uint(p.Deg() - 8),
		value:    0,
		window:   make([]byte, 0, rollinghash.DefaultWindowCap),
		oldest:   0,
	}
	res.updateTables()
	return res
}

// New returns a RabinKarp64 digest from the default polynomial obtained
// when using RandomPolynomial with the seed 1.
func New() *RabinKarp64 {
	p, err := RandomPolynomial(1)
	if err != nil {
		panic(err)
	}
	return NewFromPol(p)
}

// Reset resets the running hash to its initial state
func (d *RabinKarp64) Reset() {
	d.tables = nil
	d.value = 0
	d.window = d.window[:0]
	d.oldest = 0
	d.updateTables()
}

// Size is 8 bytes
func (d *RabinKarp64) Size() int { return Size }

// BlockSize is 1 byte
func (d *RabinKarp64) BlockSize() int { return 1 }

// WriteWindow writes the contents of the current window to w.
func (d *RabinKarp64) WriteWindow(w io.Writer) (n int, err error) {
	return window.Write(w, d.window, d.oldest)
}

// Write appends data to the rolling window and updates the digest.
func (d *RabinKarp64) Write(data []byte) (int, error) {
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

	d.value = 0
	for _, b := range d.window {
		d.value <<= 8
		d.value |= Pol(b)
		d.value = d.value.Mod(d.pol)
	}

	d.updateTables()

	return len(data), nil
}

// Sum64 returns the hash as a uint64
func (d *RabinKarp64) Sum64() uint64 {
	return uint64(d.value)
}

// Sum returns the hash as byte slice
func (d *RabinKarp64) Sum(b []byte) []byte {
	v := d.Sum64()
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the entering byte. You
// MUST initialize a window with Write() before calling this method.
func (d *RabinKarp64) Roll(c byte) {
	// This check costs 10-15% performance. If we disable it, we crash
	// when the window is empty. If we enable it, we are always correct
	// (an empty window never changes no matter how much you roll it).
	//if len(d.window) == 0 {
	//	return
	//}
	// extract the entering/leaving bytes and update the circular buffer.
	leave := uint64(d.window[d.oldest])
	d.window[d.oldest] = c
	d.oldest += 1
	if d.oldest >= len(d.window) {
		d.oldest = 0
	}

	// Work on locals so the compiler loads d.tables once and writes
	// d.value back only once. The & 63 lets it prove the shift count is
	// in range and skip the shift-by->=64 masking guard (polShift is the
	// polynomial degree minus 8, always well below 64).
	t := d.tables
	value := d.value ^ t.out[leave]
	index := byte(value >> (d.polShift & 63))
	value = value<<8 | Pol(c)
	value ^= t.mod[index]
	d.value = value
}

// BulkRoll computes the rolling checksum of every window-sized slice of data
// in one pass and writes them to dst, which must have len(data)-window+1
// elements: dst[i] is the checksum of data[i:i+window]. It is equivalent to
// Write(data[:window]) followed by a Roll for each subsequent byte, recording
// Sum64 after each step, but it indexes the leaving byte directly (data[i])
// instead of keeping a circular window and rolls two independent lanes so
// their table-lookup chains overlap in the pipeline. BulkRoll does not modify
// the receiver; it reads d.pol and fetches the (cached) tables for window.
func (d *RabinKarp64) BulkRoll(dst []uint64, data []byte, window int) {
	if window <= 0 || len(data) < window {
		return
	}
	pol := d.pol
	t := tablesFor(pol, window) // tables are window-size specific
	shift := d.polShift & 63

	n := len(data) - window // highest output index; there are n+1 outputs.

	// Lane A owns dst[0:half], lane B owns dst[half:n+1]; the extra output
	// of an odd count goes to A.
	half := (n + 2) / 2

	// Lane A warmup: Horner reduction over data[0:window].
	var vA Pol
	for j := range window {
		vA = (vA<<8 | Pol(data[j])).Mod(pol)
	}
	dst[0] = uint64(vA)

	if half > n {
		// Only one output (n == 0), or nothing left for a second lane.
		for ia := range n {
			vA ^= t.out[data[ia]]
			vA = (vA<<8 | Pol(data[ia+window])) ^ t.mod[byte(vA>>shift)]
			dst[ia+1] = uint64(vA)
		}
		return
	}

	// Lane B warmup: Horner reduction over data[half:half+window].
	var vB Pol
	for j := range window {
		vB = (vB<<8 | Pol(data[half+j])).Mod(pol)
	}
	dst[half] = uint64(vB)

	// Step both lanes in lockstep; vA and vB are independent locals so the
	// compiler keeps them in registers and the two table-lookup chains pipeline.
	ia, ib := 0, half
	for ia < half-1 && ib < n {
		vA ^= t.out[data[ia]]
		vA = (vA<<8 | Pol(data[ia+window])) ^ t.mod[byte(vA>>shift)]
		dst[ia+1] = uint64(vA)

		vB ^= t.out[data[ib]]
		vB = (vB<<8 | Pol(data[ib+window])) ^ t.mod[byte(vB>>shift)]
		dst[ib+1] = uint64(vB)

		ia++
		ib++
	}
	// Finish whichever lane is longer (A, by at most one output).
	for ; ia < half-1; ia++ {
		vA ^= t.out[data[ia]]
		vA = (vA<<8 | Pol(data[ia+window])) ^ t.mod[byte(vA>>shift)]
		dst[ia+1] = uint64(vA)
	}
	for ; ib < n; ib++ {
		vB ^= t.out[data[ib]]
		vB = (vB<<8 | Pol(data[ib+window])) ^ t.mod[byte(vB>>shift)]
		dst[ib+1] = uint64(vB)
	}
}

// BulkBoundaries reports the window positions where the rolling checksum
// satisfies sum & mask == 0, fusing the test into the hashing loop (see
// the boundary fast path). It mirrors BulkRoll exactly, replacing each
// "dst[i] = uint64(v)" with the masked test. It does not modify the receiver.
func (d *RabinKarp64) BulkBoundaries(a, b []int32, data []byte, window int, mask uint64) (na, nb int) {
	if window <= 0 || len(data) < window {
		return 0, 0
	}
	pol := d.pol
	t := tablesFor(pol, window)
	shift := d.polShift & 63

	n := len(data) - window
	half := (n + 2) / 2

	var vA Pol
	for j := range window {
		vA = (vA<<8 | Pol(data[j])).Mod(pol)
	}
	if uint64(vA)&mask == 0 {
		a[na] = 0
		na++
	}

	if half > n {
		for ia := range n {
			vA ^= t.out[data[ia]]
			vA = (vA<<8 | Pol(data[ia+window])) ^ t.mod[byte(vA>>shift)]
			if uint64(vA)&mask == 0 {
				a[na] = int32(ia + 1)
				na++
			}
		}
		return na, 0
	}

	var vB Pol
	for j := range window {
		vB = (vB<<8 | Pol(data[half+j])).Mod(pol)
	}
	if uint64(vB)&mask == 0 {
		b[nb] = int32(half)
		nb++
	}

	ia, ib := 0, half
	for ia < half-1 && ib < n {
		vA ^= t.out[data[ia]]
		vA = (vA<<8 | Pol(data[ia+window])) ^ t.mod[byte(vA>>shift)]
		if uint64(vA)&mask == 0 {
			a[na] = int32(ia + 1)
			na++
		}

		vB ^= t.out[data[ib]]
		vB = (vB<<8 | Pol(data[ib+window])) ^ t.mod[byte(vB>>shift)]
		if uint64(vB)&mask == 0 {
			b[nb] = int32(ib + 1)
			nb++
		}

		ia++
		ib++
	}
	for ; ia < half-1; ia++ {
		vA ^= t.out[data[ia]]
		vA = (vA<<8 | Pol(data[ia+window])) ^ t.mod[byte(vA>>shift)]
		if uint64(vA)&mask == 0 {
			a[na] = int32(ia + 1)
			na++
		}
	}
	for ; ib < n; ib++ {
		vB ^= t.out[data[ib]]
		vB = (vB<<8 | Pol(data[ib+window])) ^ t.mod[byte(vB>>shift)]
		if uint64(vB)&mask == 0 {
			b[nb] = int32(ib + 1)
			nb++
		}
	}
	return na, nb
}
