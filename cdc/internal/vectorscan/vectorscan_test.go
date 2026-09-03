package vectorscan

import (
	"bytes"
	"math/rand"
	"testing"
)

// naiveMax/naiveMin/naiveIndex are the dead-simple oracles. They must agree with
// both the generic path and (on amd64) the AVX2 path for every input.
func naiveMax(d []byte) byte {
	var m byte
	for _, b := range d {
		if b > m {
			m = b
		}
	}
	return m
}

func naiveMin(d []byte) byte {
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

func naiveIndex(d []byte, t byte, op int) int {
	for i, b := range d {
		var hit bool
		switch op {
		case opGT:
			hit = b > t
		case opGE:
			hit = b >= t
		case opLT:
			hit = b < t
		case opLE:
			hit = b <= t
		}
		if hit {
			return i
		}
	}
	return len(d)
}

// withPaths runs fn once per available implementation path.
func withPaths(t *testing.T, fn func(t *testing.T)) {
	t.Helper()
	t.Run("generic", func(t *testing.T) {
		if haveAVX2Path {
			saved := getAVX2()
			setAVX2(false)
			defer setAVX2(saved)
		}
		fn(t)
	})
	if haveAVX2Path && getAVX2Default() {
		t.Run("avx2", func(t *testing.T) {
			saved := getAVX2()
			setAVX2(true)
			defer setAVX2(saved)
			fn(t)
		})
	}
}

func TestExtremes(t *testing.T) {
	withPaths(t, func(t *testing.T) {
		r := rand.New(rand.NewSource(1))
		for _, n := range []int{0, 1, 2, 7, 15, 16, 17, 31, 32, 33, 63, 64, 65, 100, 1024, 4096, 8191} {
			for range 20 {
				d := make([]byte, n)
				for i := range d {
					d[i] = byte(r.Intn(256))
				}
				if got, want := MaxByte(d), naiveMax(d); got != want {
					t.Fatalf("MaxByte(n=%d): got %d want %d", n, got, want)
				}
				if got, want := MinByte(d), naiveMin(d); got != want {
					t.Fatalf("MinByte(n=%d): got %d want %d", n, got, want)
				}
			}
		}
		// All-equal and monotone inputs stress the reduction edges.
		for _, n := range []int{1, 32, 33, 96} {
			all := bytes.Repeat([]byte{0x55}, n)
			if MaxByte(all) != 0x55 || MinByte(all) != 0x55 {
				t.Fatalf("all-0x55 n=%d: max %d min %d", n, MaxByte(all), MinByte(all))
			}
		}
	})
}

func TestRangeScan(t *testing.T) {
	withPaths(t, func(t *testing.T) {
		r := rand.New(rand.NewSource(2))
		ops := []int{opGT, opGE, opLT, opLE}
		fns := []func([]byte, byte) int{
			func(d []byte, x byte) int { return IndexGT(d, x) },
			func(d []byte, x byte) int { return IndexGE(d, x) },
			func(d []byte, x byte) int { return IndexLT(d, x) },
			func(d []byte, x byte) int { return IndexLE(d, x) },
		}
		for _, n := range []int{0, 1, 2, 15, 16, 31, 32, 33, 64, 65, 200, 1000, 4096, 8191} {
			for range 30 {
				d := make([]byte, n)
				for i := range d {
					d[i] = byte(r.Intn(256))
				}
				for _, target := range []byte{0, 1, 127, 128, 200, 254, 255, byte(r.Intn(256))} {
					for k, op := range ops {
						if got, want := fns[k](d, target), naiveIndex(d, target, op); got != want {
							t.Fatalf("op=%d n=%d target=%d: got %d want %d", op, n, target, got, want)
						}
					}
				}
			}
		}
	})
}
