package vectorscan

import (
	"math/rand"
	"testing"
)

func benchData(n int) []byte {
	d := make([]byte, n)
	r := rand.New(rand.NewSource(99))
	for i := range d {
		d[i] = byte(r.Intn(256))
	}
	return d
}

func BenchmarkMaxByte(b *testing.B) {
	for _, n := range []int{1 << 10, 4 << 10, 8 << 10} {
		d := benchData(n)
		b.Run(sizeName(n), func(b *testing.B) {
			b.SetBytes(int64(n))
			for range b.N {
				sinkByte = MaxByte(d)
			}
		})
	}
}

func BenchmarkIndexGE(b *testing.B) {
	for _, n := range []int{1 << 10, 4 << 10, 8 << 10} {
		d := benchData(n)
		b.Run(sizeName(n), func(b *testing.B) {
			b.SetBytes(int64(n))
			for range b.N {
				// target 0xff: the scan runs the whole region most of the time.
				sinkInt = IndexGE(d, 0xff)
			}
		})
	}
}

var (
	sinkByte byte
	sinkInt  int
)

func sizeName(n int) string {
	switch {
	case n >= 1<<20:
		return string(rune('0'+n>>20)) + "M"
	case n >= 1<<10:
		return string(rune('0'+n>>10)) + "K"
	default:
		return "small"
	}
}
