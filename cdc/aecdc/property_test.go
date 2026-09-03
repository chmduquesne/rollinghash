package aecdc_test

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/chmduquesne/rollinghash/v4/cdc/aecdc"
)

// TestChunkerPropertyVsReference hammers the vectorscan-backed cut against refAE
// over many randomly sized buffers and min/max pairs, including the awkward
// small-min and near-boundary lengths the static cases don't reach. refAE is the
// plain byte-loop specification; the two must never disagree.
func TestChunkerPropertyVsReference(t *testing.T) {
	r := rand.New(rand.NewSource(0xAE))
	for iter := range 4000 {
		minSize := 1 + r.Intn(40)
		maxSize := minSize + 1 + r.Intn(400)
		n := r.Intn(4 * maxSize)
		data := make([]byte, n)
		// A small alphabet makes ties and equal-to-max windows common.
		alpha := 1 + r.Intn(6)
		for i := range data {
			data[i] = byte(r.Intn(alpha))
		}

		want := refAE(data, minSize, maxSize)
		got := collect(t, aecdc.New(bytes.NewReader(data), minSize, maxSize))
		if !sameChunks(got, want) {
			t.Fatalf("iter=%d min=%d max=%d n=%d alpha=%d:\n got=%v\nwant=%v",
				iter, minSize, maxSize, n, alpha, lens(got), lens(want))
		}
	}
}
