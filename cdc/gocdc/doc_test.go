package gocdc_test

import (
	"bytes"
	"fmt"
	"io"
	"log"

	"github.com/chmduquesne/rollinghash/v4/cdc/gocdc"
)

// gocdc.NewChunker mirrors PlakarKorp/go-cdc-chunkers' chunkers.NewChunker
// signature exactly: pick an algorithm by name, then drain it with Next until
// io.EOF. Migrating from that package is just changing the import path.
func ExampleNewChunker() {
	data := make([]byte, 64*1024)
	x := uint32(1)
	for i := range data {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		data[i] = byte(x)
	}

	c, err := gocdc.NewChunker("fastcdc", bytes.NewReader(data), &gocdc.ChunkerOpts{
		MinSize: 1024, NormalSize: 4096, MaxSize: 16384,
	})
	if err != nil {
		log.Fatal(err)
	}

	total, chunks := 0, 0
	for {
		chunk, err := c.Next()
		total += len(chunk)
		if len(chunk) > 0 {
			chunks++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("split %d bytes into %d chunks\n", total, chunks)
	// Output:
	// split 65536 bytes into 11 chunks
}
