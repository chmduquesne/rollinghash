//go:build amd64 && !purego

package vectorscan

// useAVX2 gates the vector path. It is a var, not a const, so the test can flip
// it off and exercise the generic fallback on the same machine.
var useAVX2 = detectAVX2()

func cpuid(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)
func xgetbv() (eax, edx uint32)

func detectAVX2() bool {
	maxID, _, _, _ := cpuid(0, 0)
	if maxID < 7 {
		return false
	}
	_, _, ecx1, _ := cpuid(1, 0)
	const osxsave, avx = 1 << 27, 1 << 28
	if ecx1&osxsave == 0 || ecx1&avx == 0 {
		return false
	}
	// XCR0 bits 1 (SSE) and 2 (AVX) must be set for the OS to preserve YMM state.
	if eax, _ := xgetbv(); eax&0x6 != 0x6 {
		return false
	}
	_, ebx7, _, _ := cpuid(7, 0)
	const avx2 = 1 << 5
	return ebx7&avx2 != 0
}

func maxByteAVX2(d []byte) byte
func minByteAVX2(d []byte) byte
func indexCmpAVX2(d []byte, target byte, op uint64) int

func maxByte(d []byte) byte {
	if useAVX2 {
		return maxByteAVX2(d)
	}
	return maxByteGeneric(d)
}

func minByte(d []byte) byte {
	if useAVX2 {
		return minByteAVX2(d)
	}
	return minByteGeneric(d)
}

func indexCmp(d []byte, t byte, op int) int {
	if useAVX2 {
		return indexCmpAVX2(d, t, uint64(op))
	}
	return indexCmpGeneric(d, t, op)
}
