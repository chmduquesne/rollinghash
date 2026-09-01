package jumpchunker_test

import (
	"bytes"
	"fmt"
	"log"

	"github.com/chmduquesne/rollinghash/v4/cdc/jumpchunker"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
)

// Jump Chunking is a Content Defined Chunking algorithm. Unlike
// rollinghash.Chunker, it uses a windowless accumulating fingerprint with a
// dual-mask trick to skip large regions of data, achieving higher throughput
// at the cost of producing different chunk boundaries. Currently only
// gearhash64 supports it.
func ExampleChunker() {
	// Repeatable pseudo-random data (xorshift), so the boundaries are stable.
	data := make([]byte, 4096)
	x := uint32(1)
	for i := range data {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		data[i] = byte(x)
	}

	// Target ~256-byte chunks, keeping each between 64 and 1024 bytes.
	// The boundary mask is derived from normalSize internally.
	c := jumpchunker.New(bytes.NewReader(data), gearhash64.New(), 256, 64, 1024)

	total := 0
	chunks := 0
	for c.Next() {
		total += len(c.Bytes())
		chunks++
	}
	if err := c.Err(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("split %d bytes into %d chunks\n", total, chunks)
	// Output:
	// split 4096 bytes into 16 chunks
}

// ExampleChunkWriter shows the push-based counterpart: feed bytes with Write as
// they arrive, drain Next in between, then Close and drain the tail.
func ExampleChunkWriter() {
	data := make([]byte, 4096)
	x := uint32(1)
	for i := range data {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		data[i] = byte(x)
	}

	w := jumpchunker.NewChunkWriter(gearhash64.New(), 256, 64, 1024)

	total, chunks := 0, 0
	drain := func() {
		for w.Next() {
			total += len(w.Bytes())
			chunks++
		}
	}
	for off := 0; off < len(data); off += 512 {
		end := off + 512
		if end > len(data) {
			end = len(data)
		}
		if _, err := w.Write(data[off:end]); err != nil {
			log.Fatal(err)
		}
		drain()
	}
	if err := w.Close(); err != nil {
		log.Fatal(err)
	}
	drain()
	if err := w.Err(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("split %d bytes into %d chunks\n", total, chunks)
	// Output:
	// split 4096 bytes into 16 chunks
}
