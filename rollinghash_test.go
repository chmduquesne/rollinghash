package rollinghash_test

import (
	"hash"
	"io/ioutil"
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

// Gets the hash sum as a uint64
func sum64(h hash.Hash) (res uint64) {
	buf := make([]byte, 0, 8)
	s := h.Sum(buf)
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

	h.Reset()
	h.Write(q[:len(q)-1])
	h.Roll(q[len(q)-1])
	return sum64(h)
}

// Compute the hash the classic way
func SumByWriteOnly(h hash.Hash, b []byte) uint64 {
	h.Reset()
	h.Write(b)
	return sum64(h)
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
		sumc := sum64(classic)
		sumr := sum64(rolling)
		if sumc != sumr {
			t.Errorf("[%s] %v: expected %x, got %x",
				hashname, s[i-n+1:i+1], sumc, sumr)
		}
	}
}

func rollEmptyWindow(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("[%s] Rolling an empty window should cause a panic", hashname)
		}
	}()
	// This should panic
	rolling.Roll(byte('x'))
}

func writeTwice(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	rolling.Write([]byte("hello "))
	rolling.Write([]byte("world"))

	classic.Write([]byte("hello world"))

	if sum64(rolling) != sum64(classic) {
		t.Errorf("[%s] Expected same results on rolling and classic", hashname)
	}
}

func writeRollWrite(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	rolling.Write([]byte(" hello"))
	rolling.Roll(byte(' '))
	rolling.Write([]byte("world"))

	classic.Write([]byte("hello world"))

	if sum64(rolling) != sum64(classic) {
		t.Errorf("[%s] Expected same results on rolling and classic", hashname)
	}
}

func writeThenWriteNothing(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	rolling.Write([]byte("hello"))
	rolling.Write([]byte(""))

	classic.Write([]byte("hello"))

	if sum64(rolling) != sum64(classic) {
		t.Errorf("[%s] Expected same results on rolling and classic", hashname)
	}
}

func writeNothing(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	rolling.Write([]byte(""))

	if sum64(rolling) != sum64(classic) {
		t.Errorf("[%s] Expected same results on rolling and classic", hashname)
	}
}

func read(t *testing.T, hashname string, rolling rollinghash.Hash) {
	rolling.Write([]byte("hello "))
	rolling.Roll(byte('w'))

	window, _ := ioutil.ReadAll(rolling)

	if string(window) != "ello w" {
		t.Errorf("[%s] Unexpect read from the hash", hashname)
	}
}

func TestFoxDog(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		foxDog(t, h.name, h.classic, h.rolling)
	}
}

func TestBlackBox(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		blackBox(t, h.name, h.classic, h.rolling)
	}
}

func TestRollEmptyWindow(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		rollEmptyWindow(t, h.name, h.classic, h.rolling)
	}
}

func TestwriteTwice(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		writeTwice(t, h.name, h.classic, h.rolling)
	}
}

func TestwriteRollWrite(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		writeRollWrite(t, h.name, h.classic, h.rolling)
	}
}

func TestWriteThenWriteNothing(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		writeThenWriteNothing(t, h.name, h.classic, h.rolling)
	}
}

func TestWriteNothing(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		writeNothing(t, h.name, h.classic, h.rolling)
	}
}

func TestRead(t *testing.T) {
	for _, h := range allHashes {
		h.rolling.Reset()
		read(t, h.name, h.rolling)
	}
}
