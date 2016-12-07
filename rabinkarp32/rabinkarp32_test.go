package rabinkarp32_test

import (
	"hash"
	"math/rand"
	"testing"

	"github.com/chmduquesne/rollinghash"
	rollsum "github.com/chmduquesne/rollinghash/rabinkarp32"
)

func NewRollingHash() rollinghash.Hash32 {
	return rollsum.New()
}

// This is a no-op to prove that we implement hash.Hash32
var _ = hash.Hash32(NewRollingHash())

func Sum32ByWriteAndRoll(b []byte) uint32 {
	q := []byte(" ")
	q = append(q, b...)
	roll := NewRollingHash()
	roll.Write(q[:len(q)-1])
	roll.Roll(q[len(q)-1])
	return roll.Sum32()
}

func Sum32ByWriteOnly(b []byte) uint32 {
	roll := NewRollingHash()
	roll.Write(b)
	return roll.Sum32()
}

func RandomBytes() (res []byte) {
	n := rand.Intn(8192)
	res = make([]byte, n)
	rand.Read(res)
	return res
}

// Commented for now, does not pass yet
func TestBlackBox(t *testing.T) {
	for i := 0; i < 1000; i++ {
		in := RandomBytes()
		if len(in) > 0 {
			wr := Sum32ByWriteAndRoll(in)
			wo := Sum32ByWriteOnly(in)

			if wo != wr {
				t.Errorf("Expected 0x%x, got 0x%x", wo, wr)
			}
		}
	}
}

func BenchmarkRollingKB(b *testing.B) {
	b.SetBytes(1024)
	window := make([]byte, 1024)
	for i := range window {
		window[i] = byte(i)
	}

	h := NewRollingHash()
	in := make([]byte, 0, h.Size())

	b.ResetTimer()
	h.Write(window)
	for i := 0; i < b.N; i++ {
		h.Roll(byte(1024 + i))
		h.Sum(in)
	}
}

func BenchmarkRolling128B(b *testing.B) {
	b.SetBytes(1024)
	window := make([]byte, 128)
	for i := range window {
		window[i] = byte(i)
	}

	h := NewRollingHash()
	in := make([]byte, 0, h.Size())

	b.ResetTimer()
	h.Write(window)
	for i := 0; i < b.N; i++ {
		h.Roll(byte(128 + i))
		h.Sum(in)
	}
}
