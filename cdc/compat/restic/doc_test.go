package restic_test

import (
	"bytes"
	"fmt"
	"io"
	"log"

	restic "github.com/chmduquesne/rollinghash/v4/cdc/compat/restic"
)

// restic.New mirrors github.com/restic/chunker's New: pick a polynomial, drain
// the chunker with Next until io.EOF. Migrating from that package is just
// changing the import path.
func ExampleNew() {
	data := make([]byte, 48*1024)
	x := uint32(1)
	for i := range data {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		data[i] = byte(x)
	}

	c := restic.New(bytes.NewReader(data), restic.Pol(0x3DA3358B4DC173),
		restic.WithBoundaries(512, 8192), restic.WithAverageBits(10))

	total, chunks := 0, 0
	for {
		chunk, err := c.Next(nil)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		total += int(chunk.Length)
		chunks++
	}
	fmt.Printf("split %d bytes into %d chunks\n", total, chunks)
	// Output:
	// split 49152 bytes into 36 chunks
}
