package repmaxsfxcdc_test

import (
	"bytes"
	"fmt"
	"log"

	"github.com/chmduquesne/rollinghash/v4/cdc/repmaxsfxcdc"
)

func ExampleChunker() {
	// Repeatable pseudo-random data (xorshift), so boundaries are stable.
	data := make([]byte, 64*1024)
	x := uint32(1)
	for i := range data {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		data[i] = byte(x)
	}

	// minSize 1 KiB, horizon 4 KiB: every chunk is in [1024, 2048).
	c := repmaxsfxcdc.New(bytes.NewReader(data), 1024, 4*1024)

	total, chunks, maxLen := 0, 0, 0
	for c.Next() {
		total += len(c.Bytes())
		chunks++
		if len(c.Bytes()) > maxLen {
			maxLen = len(c.Bytes())
		}
	}
	if err := c.Err(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("split %d bytes into %d chunks, longest %d (< 2048)\n", total, chunks, maxLen)
	// Output:
	// split 65536 bytes into 49 chunks, longest 1987 (< 2048)
}
