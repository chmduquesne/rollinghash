package gearscan

import (
	"math/rand"
	"testing"
)

var testTable [256]uint64

func init() {
	r := rand.New(rand.NewSource(1))
	for i := range testTable {
		testTable[i] = r.Uint64()
	}
}

// maxScalar is the plain one-byte-at-a-time reference Max is an unrolled port of.
func maxScalar(g *[256]uint64, data []byte, lo, hi int, fp uint64) (bestOff int, bestFP uint64) {
	bestFP = fp
	for i := lo; i < hi; i++ {
		if fp = (fp << 1) + g[data[i]]; bestFP < fp {
			bestFP, bestOff = fp, i-lo+1
		}
	}
	return
}

func TestMaxMatchesScalar(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	for trial := 0; trial < 500; trial++ {
		n := 1 + r.Intn(4000)
		data := make([]byte, n)
		for i := range data {
			data[i] = byte(r.Uint64())
		}
		lo := r.Intn(n + 1)
		for _, seed := range []uint64{0, 1, 0xdeadbeefcafef00d, ^uint64(0)} {
			wantOff, wantFP := maxScalar(&testTable, data, lo, n, seed)
			gotOff, gotFP := Max(&testTable, data, lo, n, seed)
			if gotOff != wantOff || gotFP != wantFP {
				t.Fatalf("trial %d n=%d lo=%d seed=%#x: Max=(%d,%#x) scalar=(%d,%#x)",
					trial, n, lo, seed, gotOff, gotFP, wantOff, wantFP)
			}
		}
	}
}

// TestMaxAllZeros: a run where no byte beats the seed leaves bestOff at 0.
func TestMaxNoNewMaximum(t *testing.T) {
	data := make([]byte, 100) // g[0] is small; a large seed is never beaten
	off, fp := Max(&testTable, data, 0, len(data), ^uint64(0))
	if off != 0 {
		t.Fatalf("bestOff = %d, want 0", off)
	}
	if want := (maxScalarFP(&testTable, data, ^uint64(0))); fp != want {
		t.Fatalf("bestFP = %#x, want %#x", fp, want)
	}
}

func maxScalarFP(g *[256]uint64, data []byte, fp uint64) uint64 {
	best := fp
	for _, b := range data {
		if fp = (fp << 1) + g[b]; best < fp {
			best = fp
		}
	}
	return best
}
