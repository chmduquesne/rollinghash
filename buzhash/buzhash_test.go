package buzhash_test

import (
	"hash"
	"math/rand"
	"testing"

	"github.com/chmduquesne/rollinghash"
	rollsum "github.com/chmduquesne/rollinghash/buzhash"
)

func NewRollingHash(b *[256]uint32) rollinghash.Hash32 {
	if b == nil {
		return rollsum.New()
	} else {
		return rollsum.NewFromByteArray(*b)
	}
}

// This is a no-op to prove that we implement hash.Hash32
var _ = hash.Hash32(rollsum.New())

func Sum32ByWriteAndRoll(b []byte, a *[256]uint32) uint32 {
	q := []byte(" ")
	q = append(q, b...)
	roll := NewRollingHash(a)
	roll.Write(q[:len(q)-1])
	roll.Roll(q[len(q)-1])
	return roll.Sum32()
}

func Sum32ByWriteOnly(b []byte, a *[256]uint32) uint32 {
	roll := NewRollingHash(a)
	roll.Write(b)
	return roll.Sum32()
}

func RandomBytes() (res []byte) {
	n := rand.Intn(8192)
	res = make([]byte, n)
	rand.Read(res)
	return res
}

func RandomTable() *[256]uint32 {
	mp := make(map[uint32]bool)
	var ta [256]uint32
	var j int
	for {
		x := uint32(rand.Int63())
		if !mp[x] {
			ta[j] = x
			j++
			mp[x] = true
			break
		}
	}
	return &ta
}

func TestBlackBox(t *testing.T) {
	var ta *[256]uint32
	for i := 0; i < 1000; i++ {
		for j := 0; j < 2; j++ {
			in := RandomBytes()
			if j == 1 {
				ta = RandomTable()
			} else {
				ta = nil
			}
			if len(in) > 0 {
				sum := Sum32ByWriteAndRoll(in, ta)
				ref := Sum32ByWriteOnly(in, ta)

				if ref != sum {
					t.Errorf("Expected 0x%x, got 0x%x", ref, sum)
				}
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

	h := NewRollingHash(nil)
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

	h := NewRollingHash(nil)
	in := make([]byte, 0, h.Size())

	b.ResetTimer()
	h.Write(window)
	for i := 0; i < b.N; i++ {
		h.Roll(byte(128 + i))
		h.Sum(in)
	}
}
