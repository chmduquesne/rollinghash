// Package rollinghash/buzhash implements buzhash as described by
// https://en.wikipedia.org/wiki/Rolling_hash#Cyclic_polynomial
//
// CAVEAT: avoid window lengths that are a multiple of 64 (the word size).
// buzhash rolls the sum by rotating a 64-bit word one bit per byte, so
// after 64 bytes the rotation wraps. A run of >=window identical bytes
// (very common in binary data: zero padding, 0xff flash padding,
// alignment) then collapses the hash to a single degenerate value
// (all-ones for odd multiples of 64, zero for even multiples), losing all
// entropy. With a 64-byte window over typical executables this makes the
// hash equal 0xffffffffffffffff about 1% of the time. Any window length
// that is not a multiple of 64 avoids this. This is inherent to the cyclic
// polynomial construction and cannot be fixed by changing the byte table.

package buzhash64

import (
	"io"
	"math/bits"
	"math/rand"

	"github.com/chmduquesne/rollinghash/v4"
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
	sum     uint64
	nRotate uint

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window   []byte
	oldest   int
	bytehash [256]uint64
	// rotBH caches bytehash[i] rotated left by nRotate, eliminating the
	// variable-count ROLQ CL instruction from the Roll hot path.
	rotBH [256]uint64
}

// Reset resets the Hash to its initial state.
func (d *Buzhash64) Reset() {
	d.window = d.window[:0]
	d.oldest = 0
	d.sum = 0
}

// GenerateHashes generates a list of hashes to use with buzhash
func GenerateHashes(seed int64) (res [256]uint64) {
	random := rand.New(rand.NewSource(seed))
	used := make(map[uint64]bool)
	for i := range res {
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
		window:   make([]byte, 0, rollinghash.DefaultWindowCap),
		oldest:   0,
		bytehash: b,
	}
}

// Size is 8 bytes
func (d *Buzhash64) Size() int { return Size }

// BlockSize is 1 byte
func (d *Buzhash64) BlockSize() int { return 1 }

// WriteWindow writes the contents of the current window to w.
func (d *Buzhash64) WriteWindow(w io.Writer) (n int, err error) {
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
func (d *Buzhash64) Write(data []byte) (int, error) {
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

	d.sum = 0
	for _, c := range d.window {
		d.sum = bits.RotateLeft64(d.sum, 1)
		d.sum ^= d.bytehash[int(c)]
	}
	d.nRotate = uint(len(d.window)) % 64
	for i, h := range d.bytehash {
		d.rotBH[i] = bits.RotateLeft64(h, int(d.nRotate))
	}
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
func (d *Buzhash64) Roll(c byte) {
	// This check costs 10-15% performance. If we disable it, we crash
	// when the window is empty. If we enable it, we are always correct
	// (an empty window never changes no matter how much you roll it).
	//if len(d.window) == 0 {
	//	return
	//}

	// extract the entering/leaving bytes and update the circular buffer.
	hn := d.bytehash[int(c)]
	h0 := d.rotBH[int(d.window[d.oldest])]

	d.window[d.oldest] = c
	l := len(d.window)
	d.oldest += 1
	if d.oldest >= l {
		d.oldest = 0
	}

	d.sum = bits.RotateLeft64(d.sum, 1) ^ h0 ^ hn
}

// Compile-time check that we implement the bulk fast paths.
var (
	_ rollinghash.BulkRoller     = (*Buzhash64)(nil)
	_ rollinghash.BoundaryRoller = (*Buzhash64)(nil)
)

// BulkRoll computes the rolling checksum of every window-sized slice of data
// in one pass and writes them to dst, which must have len(data)-window+1
// elements: dst[i] is the checksum of data[i:i+window]. It is equivalent to
// Write(data[:window]) followed by a Roll for each subsequent byte, recording
// Sum64 after each step, but it indexes the leaving byte directly (data[i])
// instead of keeping a circular window and rolls two independent lanes so
// their rotate/XOR chains overlap in the pipeline. BulkRoll does not modify
// the receiver; only d.bytehash is read.
func (d *Buzhash64) BulkRoll(dst []uint64, data []byte, window int) {
	if window <= 0 || len(data) < window {
		return
	}
	bh := &d.bytehash
	nRotate := window % 64

	// Precompute the rotated leaving-byte table once. This eliminates the
	// variable-count ROLQ CL instruction from the inner loop, which otherwise
	// forces nRotate into the CL register every iteration and causes the
	// compiler to spill other live values to the stack.
	var rotBH [256]uint64
	for i, h := range bh {
		rotBH[i] = bits.RotateLeft64(h, nRotate)
	}

	n := len(data) - window // highest output index; there are n+1 outputs.

	// Reslice so the compiler can prove bounds without a chain through half
	// and window, eliminating the per-iteration bounds checks.
	leaving  := data[:n+1]
	entering := data[window:]

	// Lane A owns dst[0:half], lane B owns dst[half:n+1]; the extra output
	// of an odd count goes to A.
	half := (n + 2) / 2

	// Lane A warmup over data[0:window].
	var vA uint64
	for j := range window {
		vA = bits.RotateLeft64(vA, 1) ^ bh[data[j]]
	}
	dst[0] = vA

	if half > n {
		// Only one output (n == 0), or nothing left for a second lane.
		for ia := range n {
			vA = bits.RotateLeft64(vA, 1) ^ rotBH[leaving[ia]] ^ bh[entering[ia]]
			dst[ia+1] = vA
		}
		return
	}

	// Lane B warmup over data[half:half+window].
	var vB uint64
	for j := range window {
		vB = bits.RotateLeft64(vB, 1) ^ bh[data[half+j]]
	}
	dst[half] = vB

	// Step both lanes in lockstep; vA and vB are independent locals so the
	// compiler keeps them in registers and the two rotate/XOR chains pipeline.
	ia, ib := 0, half
	for ia < half-1 && ib < n {
		vA = bits.RotateLeft64(vA, 1) ^ rotBH[leaving[ia]] ^ bh[entering[ia]]
		dst[ia+1] = vA
		vB = bits.RotateLeft64(vB, 1) ^ rotBH[leaving[ib]] ^ bh[entering[ib]]
		dst[ib+1] = vB
		ia++
		ib++
	}
	// Finish whichever lane is longer (A, by at most one output).
	for ; ia < half-1; ia++ {
		vA = bits.RotateLeft64(vA, 1) ^ rotBH[leaving[ia]] ^ bh[entering[ia]]
		dst[ia+1] = vA
	}
	for ; ib < n; ib++ {
		vB = bits.RotateLeft64(vB, 1) ^ rotBH[leaving[ib]] ^ bh[entering[ib]]
		dst[ib+1] = vB
	}
}

// BulkBoundaries reports the window positions where the rolling checksum
// satisfies sum & mask == 0, fusing the test into the hashing loop (see
// rollinghash.BoundaryRoller). It does not modify the receiver.
//
// Boundary hits are accumulated as bits in a uint64 (low 32 = lane A,
// high 32 = lane B) and extracted with TrailingZeros outside the hot loop,
// keeping the branch-heavy write-to-slice path out of every iteration.
// The inner loop is 2x unrolled so table loads for step k+1 can issue
// while computing step k.
func (d *Buzhash64) BulkBoundaries(a, b []int32, data []byte, window int, mask uint64) (na, nb int) {
	if window <= 0 || len(data) < window {
		return 0, 0
	}
	bh := &d.bytehash
	nRotate := window % 64

	var rotBH [256]uint64
	for i, h := range bh {
		rotBH[i] = bits.RotateLeft64(h, nRotate)
	}

	n := len(data) - window
	leaving  := data[:n+1]
	entering := data[window:]
	half := (n + 2) / 2

	var vA uint64
	for j := range window {
		vA = bits.RotateLeft64(vA, 1) ^ bh[data[j]]
	}
	if vA&mask == 0 {
		a[na] = 0
		na++
	}

	if half > n {
		for ia := range n {
			vA = bits.RotateLeft64(vA, 1) ^ rotBH[leaving[ia]] ^ bh[entering[ia]]
			if vA&mask == 0 {
				a[na] = int32(ia + 1)
				na++
			}
		}
		return na, 0
	}

	var vB uint64
	for j := range window {
		vB = bits.RotateLeft64(vB, 1) ^ bh[data[half+j]]
	}
	if vB&mask == 0 {
		b[nb] = int32(half)
		nb++
	}

	// Process positions in blocks of blockSize. Boundary hits accumulate as
	// bits in a single uint64: low blockSize bits for lane A, high blockSize
	// bits for lane B. This keeps na, nb, a, b out of the hot inner loop.
	// The inner loop is 2x unrolled so loads for step k+1 can overlap
	// the computation of step k.
	const blockSize = 32 // must be ≤ 32 so both halves fit in one uint64
	limitA := half - 1  // ia runs [0, limitA)
	limitB := n - half  // ib_rel runs [0, limitB)
	limit := min(limitA, limitB)
	fullBlocks := limit / blockSize

	for blk := range fullBlocks {
		base := blk * blockSize
		lA := leaving[base : base+blockSize : base+blockSize]
		lB := leaving[half+base : half+base+blockSize : half+base+blockSize]
		eA := entering[base : base+blockSize : base+blockSize]
		eB := entering[half+base : half+base+blockSize : half+base+blockSize]

		var bitsAB uint64
		for k := 0; k < blockSize; k += 2 {
			vA = bits.RotateLeft64(vA, 1) ^ rotBH[lA[k]] ^ bh[eA[k]]
			vB = bits.RotateLeft64(vB, 1) ^ rotBH[lB[k]] ^ bh[eB[k]]
			if vA&mask == 0 {
				bitsAB |= 1 << uint(k)
			}
			if vB&mask == 0 {
				bitsAB |= 1 << uint(k+32)
			}
			vA = bits.RotateLeft64(vA, 1) ^ rotBH[lA[k+1]] ^ bh[eA[k+1]]
			vB = bits.RotateLeft64(vB, 1) ^ rotBH[lB[k+1]] ^ bh[eB[k+1]]
			if vA&mask == 0 {
				bitsAB |= 1 << uint(k+1)
			}
			if vB&mask == 0 {
				bitsAB |= 1 << uint(k+33)
			}
		}

		base32 := int32(base)
		halfBase := int32(half + base)
		for bA := bitsAB & 0xffffffff; bA != 0; bA &= bA - 1 {
			a[na] = base32 + int32(bits.TrailingZeros32(uint32(bA))) + 1
			na++
		}
		for bB := bitsAB >> 32; bB != 0; bB &= bB - 1 {
			b[nb] = halfBase + int32(bits.TrailingZeros64(bB)) + 1
			nb++
		}
	}

	ia := fullBlocks * blockSize
	ib := half + ia
	for ia < limitA && ib < n {
		vA = bits.RotateLeft64(vA, 1) ^ rotBH[leaving[ia]] ^ bh[entering[ia]]
		if vA&mask == 0 {
			a[na] = int32(ia + 1)
			na++
		}
		vB = bits.RotateLeft64(vB, 1) ^ rotBH[leaving[ib]] ^ bh[entering[ib]]
		if vB&mask == 0 {
			b[nb] = int32(ib + 1)
			nb++
		}
		ia++
		ib++
	}
	for ; ia < limitA; ia++ {
		vA = bits.RotateLeft64(vA, 1) ^ rotBH[leaving[ia]] ^ bh[entering[ia]]
		if vA&mask == 0 {
			a[na] = int32(ia + 1)
			na++
		}
	}
	for ; ib < n; ib++ {
		vB = bits.RotateLeft64(vB, 1) ^ rotBH[leaving[ib]] ^ bh[entering[ib]]
		if vB&mask == 0 {
			b[nb] = int32(ib + 1)
			nb++
		}
	}
	return na, nb
}
