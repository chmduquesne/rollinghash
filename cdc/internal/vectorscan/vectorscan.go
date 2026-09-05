// Package vectorscan holds the two byte-scanning primitives that the hashless,
// extremum-based content-defined chunkers reduce to, as identified by VectorCDC
// (Udayashankar et al., "Accelerating Data Chunking in Deduplication Systems
// using Vector Instructions", FAST '25 / ACM TOS 2026):
//
//   - Extreme Byte Search — the maximum or minimum byte over a region
//     ([MaxByte], [MinByte]).
//   - Range Scan — the first byte that compares a given way against a target
//     ([IndexGT], [IndexGE], [IndexLT], [IndexLE]).
//
// Unlike a rolling-hash boundary test, neither primitive has a serial byte
// dependency, so both vectorize cleanly. On amd64 with AVX2 the scan runs on
// 256-bit vectors (packed max/min, packed compare, move-mask); on arm64 it
// runs on 128-bit NEON vectors (packed max/min, packed compare, block-level
// hit test with a scalar fallback to pinpoint the index). Every other build —
// any other architecture, or the purego tag — uses the plain byte-loop
// versions in this file, which are also the behavioural specification both
// assembly paths are tested against.
//
// It is internal and not part of the public API. cdc/aecdc, cdc/ramcdc and
// cdc/maxpcdc drive their cutpoint search with it.
package vectorscan

// comparator selectors for the assembly range scan; kept in sync with the
// immediate handled in indexCmpAVX2.
const (
	opGT = 0
	opGE = 1
	opLT = 2
	opLE = 3
)

// MaxByte returns the largest byte in d, or 0 if d is empty.
func MaxByte(d []byte) byte { return maxByte(d) }

// MinByte returns the smallest byte in d, or 0 if d is empty.
func MinByte(d []byte) byte { return minByte(d) }

// IndexGT returns the index of the first byte of d strictly greater than
// target, or len(d) if there is none.
func IndexGT(d []byte, target byte) int { return indexCmp(d, target, opGT) }

// IndexGE returns the index of the first byte of d greater than or equal to
// target, or len(d) if there is none.
func IndexGE(d []byte, target byte) int { return indexCmp(d, target, opGE) }

// IndexLT returns the index of the first byte of d strictly less than target,
// or len(d) if there is none.
func IndexLT(d []byte, target byte) int { return indexCmp(d, target, opLT) }

// IndexLE returns the index of the first byte of d less than or equal to
// target, or len(d) if there is none.
func IndexLE(d []byte, target byte) int { return indexCmp(d, target, opLE) }

// maxByteGeneric is the reference Extreme Byte Search (maximum).
func maxByteGeneric(d []byte) byte {
	var m byte
	for _, b := range d {
		if b > m {
			m = b
		}
	}
	return m
}

// minByteGeneric is the reference Extreme Byte Search (minimum).
func minByteGeneric(d []byte) byte {
	if len(d) == 0 {
		return 0
	}
	m := byte(0xff)
	for _, b := range d {
		if b < m {
			m = b
		}
	}
	return m
}

// indexCmpGeneric is the reference Range Scan for every comparator.
func indexCmpGeneric(d []byte, target byte, op int) int {
	switch op {
	case opGT:
		for i, b := range d {
			if b > target {
				return i
			}
		}
	case opGE:
		for i, b := range d {
			if b >= target {
				return i
			}
		}
	case opLT:
		for i, b := range d {
			if b < target {
				return i
			}
		}
	default: // opLE
		for i, b := range d {
			if b <= target {
				return i
			}
		}
	}
	return len(d)
}
