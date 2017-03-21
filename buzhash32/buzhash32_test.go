package buzhash32_test

import (
	"hash"
	"math/rand"
	"testing"
	"time"

	"github.com/chmduquesne/rollinghash"
	rollsum "github.com/chmduquesne/rollinghash/buzhash32"
)

var testHash [256]uint32

func RandomHash() (res [256]uint32) {
	used := make(map[uint32]bool)
	for i, _ := range res {
		x := uint32(rand.Int63())
		for used[x] {
			x = uint32(rand.Int63())
		}
		used[x] = true
		res[i] = x
	}
	return res
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
	testHash = RandomHash()
}

func NewRollingHash() rollinghash.Hash32 {
	return rollsum.NewFromUint32Array(testHash)
}

// This is a no-op to prove that we implement hash.Hash32
var _ = hash.Hash32(rollsum.New())

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

func TestBlackBox(t *testing.T) {
	for i := 0; i < 1000; i++ {
		in := RandomBytes()
		if len(in) > 0 {
			sum := Sum32ByWriteAndRoll(in)
			ref := Sum32ByWriteOnly(in)

			if ref != sum {
				t.Errorf("Expected 0x%x, got 0x%x", ref, sum)
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

	h := rollsum.New()
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

	h := rollsum.New()
	in := make([]byte, 0, h.Size())

	b.ResetTimer()
	h.Write(window)
	for i := 0; i < b.N; i++ {
		h.Roll(byte(128 + i))
		h.Sum(in)
	}
}
