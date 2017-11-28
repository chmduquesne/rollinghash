package rollinghash_test

import (
	"fmt"
	"hash"
	"math/rand"
	"testing"

	"github.com/chmduquesne/rollinghash"
	_adler32 "github.com/chmduquesne/rollinghash/adler32"
	"github.com/chmduquesne/rollinghash/bozo32"
	"github.com/chmduquesne/rollinghash/buzhash32"
	"github.com/chmduquesne/rollinghash/buzhash64"
	"github.com/chmduquesne/rollinghash/rabinkarp64"
)

var allHashes = []struct {
	name    string
	classic hash.Hash
	rolling rollinghash.Hash
}{
	{"adler32", _adler32.New(), _adler32.New()},
	{"buzhash32", buzhash32.New(), buzhash32.New()},
	{"buzhash64", buzhash64.New(), buzhash64.New()},
	{"bozo32", bozo32.New(), bozo32.New()},
	{"rabinkarp64", rabinkarp64.New(), rabinkarp64.New()},
}

// Converts a byte hash into a uint64
func hash2uint64(s []byte) (res uint64) {
	if len(s) > 8 {
		panic(fmt.Errorf("Input has more than 8 bytes and does not fit in a uint64"))
	}
	for _, b := range s {
		res <<= 8
		res |= uint64(b)
	}
	return
}

// Compute the hash by creating a byte slice with an additionnal '\0' at
// the beginning, writing the slice without the last byte, and then
// rolling the last byte.
func SumByWriteAndRoll(h rollinghash.Hash, b []byte) uint64 {
	q := []byte("\x00")
	q = append(q, b...)

	buf := make([]byte, 0, 8)
	h.Reset()
	h.Write(q[:len(q)-1])
	h.Roll(q[len(q)-1])
	return hash2uint64(h.Sum(buf))
}

// Compute the hash the classic way
func SumByWriteOnly(h hash.Hash, b []byte) uint64 {
	buf := make([]byte, 0, 8)
	h.Reset()
	h.Write(b)
	return hash2uint64(h.Sum(buf))
}

// Create some random slice (length betwen 0 and 8KB, random content)
func RandomBytes() (res []byte) {
	n := rand.Intn(8192)
	res = make([]byte, n)
	rand.Read(res)
	return res
}

// Verify that, on random inputs, the classic hash and the rollinghash
// return the same values
func blackBox(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	for i := 0; i < 1000; i++ {
		in := RandomBytes()
		if len(in) > 0 {
			sum := SumByWriteAndRoll(rolling, in)
			ref := SumByWriteOnly(classic, in)

			if ref != sum {
				t.Errorf("[%s] Expected 0x%x, got 0x%x", hashname, ref, sum)
			}
		}
	}
}

// Roll a window of 16 bytes with a classic hash and a rolling hash and
// compare the results
func foxDog(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	s := []byte("The quick brown fox jumps over the lazy dog")

	// buffers to write the sums
	bufc := make([]byte, 0, 8)
	bufr := make([]byte, 0, 8)

	// Window len
	n := 16

	// Load the window into the rolling hash
	rolling.Write(s[:n])

	// Roll it and compare the result with full re-calculus every time
	for i := n; i < len(s); i++ {

		// Reset and write the window in classic
		classic.Reset()
		classic.Write(s[i-n+1 : i+1])

		// Roll the incoming byte in rolling
		rolling.Roll(s[i])

		// Compare the hashes
		sumc := hash2uint64(classic.Sum(bufc))
		sumr := hash2uint64(rolling.Sum(bufr))
		if sumc != sumr {
			t.Errorf("[%s] %v: expected %x, got %x",
				hashname, s[i-n+1:i+1], sumc, sumr)
		}
	}
}

func TestFoxDog(t *testing.T) {
	for _, h := range allHashes {
		foxDog(t, h.name, h.classic, h.rolling)
	}
}

func TestBlackBox(t *testing.T) {
	for _, h := range allHashes {
		blackBox(t, h.name, h.classic, h.rolling)
	}
}
