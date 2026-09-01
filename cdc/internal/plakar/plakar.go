// Package plakar is a faithful local model of PlakarKorp/go-cdc-chunkers: its
// shared Gear table, its per-variant mask/parameter derivation, verbatim ports
// of its three cutpoint Algorithm functions, and a driver reproducing its
// streaming Chunker.Next loop. It lets the cdc tree stay byte-for-byte
// compatible with plakar without taking a real dependency on it: cdc/gocdc
// uses the table and derivations, and the algorithm packages' tests use the
// Algorithm ports as an oracle. It is internal and not part of the public API.
//
// Sources (github.com/PlakarKorp/go-cdc-chunkers, main):
//   - chunkers/jc/jc.go
//   - chunkers/fastcdc/fastcdc.go
//   - chunkers/ultracdc/ultracdc.go
//   - chunkers.go (Chunker.Next)
package plakar

import (
	"math"
	"math/bits"
)

// Opts mirrors the fields of chunkers.ChunkerOpts that the algorithms read.
type Opts struct {
	MinSize    int
	NormalSize int
	MaxSize    int
}

// generateSpacedMask is chunkers/jc/jc.go generateSpacedMask (byte-identical to
// the fastcdc copy).
func generateSpacedMask(oneCount int, totalBits int) uint64 {
	if oneCount >= totalBits {
		return 0xFFFFFFFFFFFFFFFF
	}
	if oneCount <= 0 {
		return 0
	}
	step := totalBits / oneCount
	var mask uint64 = 0
	for i := 0; i < oneCount; i++ {
		pos := totalBits - 1 - i*step
		if pos >= 0 {
			mask |= 1 << pos
		}
	}
	return mask
}

// ---------------------------------------------------------------------------
// FastCDC  (chunkers/fastcdc/fastcdc.go)
// ---------------------------------------------------------------------------

// FastCDCLegacyMaskS and FastCDCLegacyMaskL are the hardcoded masks plakar uses
// for the "fastcdc" (legacy) variant, and for any variant at the default
// 2K/8K/64K sizes.
const (
	FastCDCLegacyMaskS = uint64(0x0003590703530000)
	FastCDCLegacyMaskL = uint64(0x0000d90003530000)
)

// FastCDCNormalLevel is plakar's hardcoded normalLevel.
const FastCDCNormalLevel = 2

// FastCDCMasks is chunkers/fastcdc/fastcdc.go calculateMasks.
func FastCDCMasks(normalSize, normalLevel int) (maskS, maskL uint64) {
	b := uint64(math.Log2(float64(normalSize)))
	sBits := b + uint64(normalLevel)
	lBits := b - uint64(normalLevel)
	maskS = generateSpacedMask(int(sBits), 64)
	maskL = generateSpacedMask(int(lBits), 64)
	return
}

// FastCDCSetupMasks reproduces the FastCDC Setup mask selection.
// legacy is true for the "fastcdc"/"kfastcdc" variants.
func FastCDCSetupMasks(o Opts, legacy bool) (maskS, maskL uint64) {
	if legacy || (o.MinSize == 2*1024 && o.MaxSize == 64*1024 && o.NormalSize == 8*1024) {
		return FastCDCLegacyMaskS, FastCDCLegacyMaskL
	}
	return FastCDCMasks(o.NormalSize, FastCDCNormalLevel)
}

// FastCDCAlgorithm is chunkers/fastcdc/fastcdc.go (*FastCDC).Algorithm.
func FastCDCAlgorithm(g *[256]uint64, maskS, maskL uint64, o Opts, data []byte, n int) int {
	MinSize, MaxSize, NormalSize := o.MinSize, o.MaxSize, o.NormalSize

	switch {
	case n <= MinSize:
		return n
	case n >= MaxSize:
		n = MaxSize
	case n <= NormalSize:
		NormalSize = n
	}

	fp, i, mask := uint64(0), MinSize, maskS
	for ; i < n; i++ {
		if i == NormalSize {
			mask = maskL
		}
		fp = (fp << 1) + g[data[i]]
		if (fp & mask) == 0 {
			return i
		}
	}
	return i
}

// ---------------------------------------------------------------------------
// JC  (chunkers/jc/jc.go)
// ---------------------------------------------------------------------------

// JCLegacyMaskC and JCLegacyMaskJ are the hardcoded masks plakar uses for the
// "jc"/"jc-v1.0.0" variants at the default 2K/8K/64K sizes (and always, for the
// legacy "jc" variant).
const (
	JCLegacyMaskC = uint64(0x590003570000)
	JCLegacyMaskJ = uint64(0x590003560000)
)

func embedMask(maskC uint64) uint64 {
	if maskC == 0 {
		return 0
	}
	return maskC & (maskC - 1)
}

// JCSetup reproduces chunkers/jc/jc.go (*JC).Setup: it derives maskC, maskJ and
// jumpLength. legacy is true for the "jc" variant.
func JCSetup(o Opts, legacy bool) (maskC, maskJ uint64, jumpLength int) {
	b := uint64(math.Log2(float64(o.NormalSize)))
	cOnes := b - 1
	jOnes := cOnes - 1
	numerator := 1 << (cOnes + jOnes)
	denominator := (1 << cOnes) - (1 << jOnes)
	jumpLength = numerator / denominator

	if legacy || (o.MinSize == 2*1024 && o.MaxSize == 64*1024 && o.NormalSize == 8*1024) {
		maskC = JCLegacyMaskC
		maskJ = JCLegacyMaskJ
	} else {
		maskC = generateSpacedMask(int(cOnes), 64)
		maskJ = embedMask(maskC)
	}
	return
}

// JCAlgorithm is chunkers/jc/jc.go (*JC).Algorithm.
func JCAlgorithm(g *[256]uint64, maskC, maskJ uint64, jumpLength int, specFaithful bool, o Opts, data []byte, n int) int {
	MinSize, MaxSize, NormalSize := o.MinSize, o.MaxSize, o.NormalSize

	switch {
	case specFaithful:
		if n >= MaxSize {
			n = MaxSize
		}
	case n <= NormalSize:
		return n
	case n >= MaxSize:
		n = MaxSize
	}

	fp := uint64(0)
	i := MinSize
	for i < n {
		fp = (fp << 1) + g[data[i]]
		if (fp & maskJ) == 0 {
			if (fp & maskC) == 0 {
				return i
			}
			fp = 0
			i = i + jumpLength
		} else {
			i++
		}
	}
	return min(i, n)
}

// ---------------------------------------------------------------------------
// UltraCDC  (chunkers/ultracdc/ultracdc.go)
// ---------------------------------------------------------------------------

var hammingDistanceTo0xAA [256]int

func init() {
	for i := range hammingDistanceTo0xAA {
		hammingDistanceTo0xAA[i] = bits.OnesCount8(byte(i) ^ 0xAA)
	}
}

// HammingTo0xAA exposes the precomputed table for the ultracdc package to
// cross-check its own copy.
func HammingTo0xAA() [256]int { return hammingDistanceTo0xAA }

// UltraCDCAlgorithm is chunkers/ultracdc/ultracdc.go (*UltraCDC).Algorithm.
func UltraCDCAlgorithm(specFaithful bool, o Opts, data []byte, n int) (cutpoint int) {
	const (
		maskS                     uint64 = 0x2F
		maskL                     uint64 = 0x2C
		lowEntropyStringThreshold int    = 64
	)

	minSize := o.MinSize
	maxSize := o.MaxSize
	normalSize := o.NormalSize

	var lowEntropyCount int
	mask := maskS

	switch {
	case n <= minSize:
		cutpoint = n
		return
	case n >= maxSize:
		n = maxSize
	case n <= normalSize:
		normalSize = n
	}

	if n < minSize+8 {
		cutpoint = n
		return
	}

	outBufWin := data[minSize : minSize+8]

	dist := 0
	for _, v := range outBufWin {
		dist += bits.OnesCount8(v ^ 0xAA)
	}

	var inBufWin []byte
	for i := minSize + 8; i <= n-8; i += 8 {
		if i >= normalSize {
			mask = maskL
		}

		inBufWin = data[i : i+8]

		if bytesEqual(inBufWin, outBufWin) {
			lowEntropyCount++
			if lowEntropyCount >= lowEntropyStringThreshold {
				cutpoint = i + 8
				return
			}
			continue
		}

		lowEntropyCount = 0
		for j := 0; j < 8; j++ {
			if (uint64(dist) & mask) == 0 {
				if specFaithful {
					cutpoint = i + 8
				} else {
					cutpoint = i + j
				}
				return
			}
			outByte := data[i+j-8]
			inByte := data[i+j]

			update := hammingDistanceTo0xAA[inByte] - hammingDistanceTo0xAA[outByte]
			dist += update
		}
		outBufWin = inBufWin
	}

	cutpoint = n
	return
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Streaming driver  (chunkers.go (*Chunker).Next / Split)
// ---------------------------------------------------------------------------

// Algorithm is the signature every ported cutpoint function reduces to once its
// parameters are bound.
type Algorithm func(data []byte, n int) int

// Chunks runs algo over the whole in-memory buffer exactly the way plakar's
// Chunker.Next drives it: peek up to MaxSize bytes, call the algorithm, emit
// data[:cutpoint], discard, repeat until the buffer is exhausted. The returned
// slices alias buf.
func Chunks(algo Algorithm, buf []byte, o Opts) [][]byte {
	var out [][]byte
	for len(buf) > 0 {
		n := len(buf)
		if n > o.MaxSize {
			n = o.MaxSize
		}
		cut := algo(buf[:n], n)
		if cut <= 0 {
			cut = n // defensive: never stall
		}
		out = append(out, buf[:cut])
		buf = buf[cut:]
	}
	return out
}
