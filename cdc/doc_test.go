package cdc_test

import (
	"bytes"
	"fmt"
	"log"

	"github.com/chmduquesne/rollinghash/v4/cdc/jumpchunker"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
)

// randData returns repeatable pseudo-random bytes (xorshift), so chunk
// boundaries are stable across runs.
func randData(n int) []byte {
	data := make([]byte, n)
	x := uint32(1)
	for i := range data {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		data[i] = byte(x)
	}
	return data
}

// Jump Chunking: a windowless accumulating fingerprint with a region-skipping
// dual-mask trick. Higher throughput than the parent package's Chunker, at the
// cost of different boundary positions.
//
// Every algorithm in this umbrella follows the same Next/Bytes/Err loop, so
// switching algorithms is a one-line change at construction.
func Example_jumpChunking() {
	data := randData(8192)

	// Target ~512-byte chunks, kept in [128, 2048].
	c := jumpchunker.New(bytes.NewReader(data), gearhash64.New(), 512, 128, 2048)

	total, chunks := 0, 0
	for c.Next() {
		total += len(c.Bytes())
		chunks++
	}
	if err := c.Err(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%d chunks, total %d bytes\n", chunks, total)
	// Output:
	// 14 chunks, total 8192 bytes
}
