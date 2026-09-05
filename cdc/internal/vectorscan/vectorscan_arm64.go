//go:build arm64 && !purego

package vectorscan

// useNEON gates the vector path. NEON is part of the arm64 baseline, so unlike
// AVX2 there is nothing to detect at runtime; the var exists only so the test
// can flip it off and exercise the generic fallback on the same machine.
var useNEON = true

func maxByteNEON(d []byte) byte
func minByteNEON(d []byte) byte
func indexCmpNEON(d []byte, target byte, op uint64) int

func maxByte(d []byte) byte {
	if useNEON {
		return maxByteNEON(d)
	}
	return maxByteGeneric(d)
}

func minByte(d []byte) byte {
	if useNEON {
		return minByteNEON(d)
	}
	return minByteGeneric(d)
}

func indexCmp(d []byte, t byte, op int) int {
	if useNEON {
		return indexCmpNEON(d, t, uint64(op))
	}
	return indexCmpGeneric(d, t, op)
}
