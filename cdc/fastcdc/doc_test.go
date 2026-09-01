package fastcdc_test

import (
	"bytes"
	"fmt"
	"log"

	"github.com/chmduquesne/rollinghash/v4/cdc/fastcdc"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
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

	c := fastcdc.New(bytes.NewReader(data), gearhash64.New(), 1024, 4096, 16384)

	total, chunks := 0, 0
	for c.Next() {
		total += len(c.Bytes())
		chunks++
	}
	if err := c.Err(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("split %d bytes into %d chunks\n", total, chunks)
	// Output:
	// split 65536 bytes into 14 chunks
}
