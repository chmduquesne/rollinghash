// Package rollinghash/gearhash64 implements the Gear rolling hash (64-bit).
// https://www.usenix.org/system/files/conference/atc16/atc16-paper-xia.pdf
//
// For a window of w bytes [b0, b1, ..., b_{w-1}] the hash is:
//
//	h = gear[b0]<<(w-1) + gear[b1]<<(w-2) + ... + gear[b_{w-1}]
//
// Rolling in byte c (dropping b0) uses: h = (h<<1) - (gear[b0]<<w) + gear[c].
// When w >= 64 the subtraction term is zero — the oldest byte's contribution
// has already been shifted out of the 64-bit word — so the formula is correct
// for all window sizes.
package gearhash64

import (
	"io"
	"math/bits"
	"math/rand"

	"github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/internal/window"
)

var defaultHashes [256]uint64

func init() {
	defaultHashes = GenerateHashes(1)
}

// Size is the size of the checksum in bytes.
const Size = 8

// GearHash64 is a digest which satisfies the rollinghash.Hash64 interface.
// It implements the Gear rolling hash used in FastCDC.
type GearHash64 struct {
	sum uint64

	// window is a circular buffer tracking the current window bytes.
	window []byte
	oldest int
	gear   [256]uint64
	// shiftedGear caches gear[i] << len(window), eliminating the
	// variable-count SHLQ CL instruction from the Roll hot path.
	shiftedGear [256]uint64
}

// GenerateHashes generates a table of 256 random 64-bit values for use with
// GearHash64.
func GenerateHashes(seed int64) (res [256]uint64) {
	r := rand.New(rand.NewSource(seed))
	for i := range res {
		res[i] = r.Uint64()
	}
	return res
}

// New returns a GearHash64 using the default table seeded with 1.
func New() *GearHash64 {
	return NewFromUint64Array(defaultHashes)
}

// NewFromUint64Array returns a GearHash64 using the provided lookup table.
func NewFromUint64Array(g [256]uint64) *GearHash64 {
	return &GearHash64{
		window: make([]byte, 0, rollinghash.DefaultWindowCap),
		gear:   g,
	}
}

// Reset resets the Hash to its initial state.
func (d *GearHash64) Reset() {
	d.window = d.window[:0]
	d.oldest = 0
	d.sum = 0
}

// Size returns 8 bytes.
func (d *GearHash64) Size() int { return Size }

// BlockSize returns 1 byte.
func (d *GearHash64) BlockSize() int { return 1 }

// WriteWindow writes the current window contents to w.
func (d *GearHash64) WriteWindow(w io.Writer) (n int, err error) {
	return window.Write(w, d.window, d.oldest)
}

// Write appends data to the rolling window and recomputes the digest. It never
// returns an error.
func (d *GearHash64) Write(data []byte) (int, error) {
	l := len(data)
	if l == 0 {
		return 0, nil
	}
	// Re-arrange the window so that the leftmost element is at index 0.
	if d.oldest != 0 {
		window.MoveLeft(d.window, d.oldest)
		d.oldest = 0
	}
	d.window = append(d.window, data...)

	d.sum = 0
	for _, c := range d.window {
		d.sum = (d.sum << 1) + d.gear[c]
	}
	// (h << w) is zero when w >= 64: the oldest byte's contribution has been
	// fully shifted out of the 64-bit accumulator, same as in Roll.
	if w := uint(len(d.window)); w < 64 {
		for i, h := range d.gear {
			d.shiftedGear[i] = h << w
		}
	} else {
		d.shiftedGear = [256]uint64{}
	}
	return len(data), nil
}

// Sum64 returns the hash as a uint64.
func (d *GearHash64) Sum64() uint64 { return d.sum }

// Sum returns the hash as a byte slice.
func (d *GearHash64) Sum(b []byte) []byte {
	v := d.Sum64()
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum as byte c enters the window and the oldest byte
// leaves. You MUST call Write before Roll.
func (d *GearHash64) Roll(c byte) {
	h0 := d.shiftedGear[d.window[d.oldest]]
	hn := d.gear[c]

	d.window[d.oldest] = c
	l := len(d.window)
	d.oldest++
	if d.oldest >= l {
		d.oldest = 0
	}

	d.sum = (d.sum << 1) - h0 + hn
}

// BatchRoll computes the rolling checksum of every window-sized slice of data
// in one pass and writes them to dst, which must have len(data)-window+1
// elements. Two independent accumulator lanes let the CPU overlap their
// dependency chains and approach the ILP ceiling of the load-latency-bound
// step h = (h<<1) - shiftedGear[leaving] + gear[entering]. It does not
// modify the receiver.
func (d *GearHash64) BatchRoll(dst []uint64, data []byte, window int) {
	if window <= 0 || len(data) < window {
		return
	}
	g := &d.gear

	// Precompute the shifted leaving-byte table: shiftedGear[b] = gear[b] << window.
	// When window >= 64 all entries are zero, which is correct.
	var shiftedGear [256]uint64
	if window < 64 {
		w := uint(window)
		for i, h := range g {
			shiftedGear[i] = h << w
		}
	}

	n := len(data) - window
	leaving := data[:n+1]
	entering := data[window:]
	half := (n + 2) / 2

	// Lane A warmup over data[0:window].
	var vA uint64
	for j := range window {
		vA = (vA << 1) + g[data[j]]
	}
	dst[0] = vA

	if half > n {
		for ia := range n {
			vA = (vA << 1) - shiftedGear[leaving[ia]] + g[entering[ia]]
			dst[ia+1] = vA
		}
		return
	}

	// Lane B warmup over data[half:half+window].
	var vB uint64
	for j := range window {
		vB = (vB << 1) + g[data[half+j]]
	}
	dst[half] = vB

	ia, ib := 0, half
	for ia < half-1 && ib < n {
		vA = (vA << 1) - shiftedGear[leaving[ia]] + g[entering[ia]]
		dst[ia+1] = vA
		vB = (vB << 1) - shiftedGear[leaving[ib]] + g[entering[ib]]
		dst[ib+1] = vB
		ia++
		ib++
	}
	for ; ia < half-1; ia++ {
		vA = (vA << 1) - shiftedGear[leaving[ia]] + g[entering[ia]]
		dst[ia+1] = vA
	}
	for ; ib < n; ib++ {
		vB = (vB << 1) - shiftedGear[leaving[ib]] + g[entering[ib]]
		dst[ib+1] = vB
	}
}

// BatchBoundaries reports the window positions where the rolling checksum
// satisfies sum & mask == 0, fusing the boundary test into the hashing loop.
// It uses the same two-lane design as BatchRoll but avoids the register
// pressure that comes from carrying na, nb, a, and b into the hot loop: each
// block of 64 positions accumulates hits as bits in a uint64, then extracts
// positions via TrailingZeros64 in a rare outer step. The inner loop is 2x
// unrolled so loads for step k+1 can issue while computing step k.
// It does not modify the receiver.
func (d *GearHash64) BatchBoundaries(a, b []int32, data []byte, window int, mask uint64) (na, nb int) {
	if window <= 0 || len(data) < window {
		return 0, 0
	}
	g := &d.gear

	var shiftedGear [256]uint64
	if window < 64 {
		w := uint(window)
		for i, h := range g {
			shiftedGear[i] = h << w
		}
	}

	n := len(data) - window
	leaving := data[:n+1]
	entering := data[window:]
	half := (n + 2) / 2

	var vA uint64
	for j := range window {
		vA = (vA << 1) + g[data[j]]
	}
	if vA&mask == 0 {
		a[na] = 0
		na++
	}

	if half > n {
		for ia := range n {
			vA = (vA << 1) - shiftedGear[leaving[ia]] + g[entering[ia]]
			if vA&mask == 0 {
				a[na] = int32(ia + 1)
				na++
			}
		}
		return na, 0
	}

	var vB uint64
	for j := range window {
		vB = (vB << 1) + g[data[half+j]]
	}
	if vB&mask == 0 {
		b[nb] = int32(half)
		nb++
	}

	// Process positions in blocks of blockSize. Boundary hits accumulate as
	// bits in a single uint64: the low blockSize bits for lane A, the high
	// blockSize bits for lane B. This keeps na, nb, a, b out of the hot inner
	// loop and reduces the two-bitmask live set from 13 words to 12 (the amd64
	// GP register limit in Go), eliminating register spills. The inner loop is
	// 2x unrolled so A and B loads can overlap each other's table-lookup
	// latency without adding extra temporaries.
	const blockSize = 32 // must be ≤ 32 so both halves fit in one uint64
	limitA := half - 1  // ia runs [0, limitA)
	limitB := n - half  // ib_rel runs [0, limitB)
	limit := min(limitA, limitB)
	fullBlocks := limit / blockSize

	for blk := range fullBlocks {
		base := blk * blockSize
		// Three-index slicing lets the compiler prove k < blockSize for the
		// inner loop and eliminate the per-element bounds checks.
		lA := leaving[base : base+blockSize : base+blockSize]
		lB := leaving[half+base : half+base+blockSize : half+base+blockSize]
		eA := entering[base : base+blockSize : base+blockSize]
		eB := entering[half+base : half+base+blockSize : half+base+blockSize]

		// Low 32 bits: lane-A hits; high 32 bits: lane-B hits.
		var bitsAB uint64
		for k := 0; k < blockSize; k += 2 {
			vA = (vA << 1) - shiftedGear[lA[k]] + g[eA[k]]
			vB = (vB << 1) - shiftedGear[lB[k]] + g[eB[k]]
			if vA&mask == 0 {
				bitsAB |= 1 << uint(k)
			}
			if vB&mask == 0 {
				bitsAB |= 1 << uint(k+32)
			}
			vA = (vA << 1) - shiftedGear[lA[k+1]] + g[eA[k+1]]
			vB = (vB << 1) - shiftedGear[lB[k+1]] + g[eB[k+1]]
			if vA&mask == 0 {
				bitsAB |= 1 << uint(k+1)
			}
			if vB&mask == 0 {
				bitsAB |= 1 << uint(k+33)
			}
		}

		// Extract boundary positions (rare: ~1 per 8K positions with mask=0x1fff).
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

	// Tail: remainder of the joint phase plus single-lane cleanups.
	ia := fullBlocks * blockSize
	ib := half + ia
	for ia < limitA && ib < n {
		vA = (vA << 1) - shiftedGear[leaving[ia]] + g[entering[ia]]
		if vA&mask == 0 {
			a[na] = int32(ia + 1)
			na++
		}
		vB = (vB << 1) - shiftedGear[leaving[ib]] + g[entering[ib]]
		if vB&mask == 0 {
			b[nb] = int32(ib + 1)
			nb++
		}
		ia++
		ib++
	}
	for ; ia < limitA; ia++ {
		vA = (vA << 1) - shiftedGear[leaving[ia]] + g[entering[ia]]
		if vA&mask == 0 {
			a[na] = int32(ia + 1)
			na++
		}
	}
	for ; ib < n; ib++ {
		vB = (vB << 1) - shiftedGear[leaving[ib]] + g[entering[ib]]
		if vB&mask == 0 {
			b[nb] = int32(ib + 1)
			nb++
		}
	}
	return na, nb
}

// JumpBoundaries finds CDC boundaries in data using the Jump Chunking
// algorithm (windowless, accumulating fingerprint). Scanning begins at
// firstSkip (the remaining min-zone bytes for the current chunk; fp is treated
// as 0 there). After each boundary the next chunk's min zone is skipped by
// advancing minStep bytes; after a false maskJ hit the speculative jump
// advances jumpLen bytes. fp is the incoming fingerprint state (0 at chunk
// start or after any jump). Boundary positions are written to a[:n]. newFp is
// the fingerprint since the last reset. When a jump or min-step crosses the end
// of data, skip is the number of bytes to carry over to the next slice (newFp
// is 0). Does not modify the receiver.
func (d *GearHash64) JumpBoundaries(a []int32, data []byte, maskC uint64, jumpLen int, fp uint64, firstSkip, minStep int) (n int, newFp uint64, skip int) {
	if len(data) == 0 || len(a) == 0 {
		return 0, fp, 0
	}
	if firstSkip >= len(data) {
		return 0, 0, firstSkip - len(data)
	}
	maskJ := maskC & (maskC - 1)
	g := &d.gear
	end := len(data)
	if firstSkip > 0 {
		fp = 0 // bytes before firstSkip are the min zone; no fp contribution
	}
	for i := firstSkip; i < end; {
		fp = (fp << 1) + g[data[i]]
		i++
		if fp&maskJ == 0 {
			atBoundary := fp&maskC == 0
			fp = 0
			if atBoundary {
				a[n] = int32(i - 1)
				n++
				if n >= len(a) {
					return n, 0, 0
				}
				i += minStep
			} else {
				i += jumpLen - 1
			}
			if i >= end {
				return n, 0, i - end
			}
		}
	}
	return n, fp, 0
}
