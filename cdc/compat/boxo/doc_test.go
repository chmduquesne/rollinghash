package boxo_test

import (
	"bytes"
	"fmt"
	"io"
	"log"

	chunk "github.com/chmduquesne/rollinghash/v4/cdc/compat/boxo"
)

// This package mirrors github.com/ipfs/boxo/chunker: pick a splitter (directly
// or with FromString), then drain it with NextBytes until io.EOF. Migrating
// from that package is just changing the import path.
func ExampleFromString() {
	data := make([]byte, 1<<20)
	var x uint64 = 0x9E3779B97F4A7C15
	for i := range data {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		data[i] = byte(x)
	}

	s, err := chunk.FromString(bytes.NewReader(data), "rabin-262144")
	if err != nil {
		log.Fatal(err)
	}

	total, chunks := 0, 0
	for {
		b, err := s.NextBytes()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		total += len(b)
		chunks++
	}
	fmt.Printf("split %d bytes into %d chunks\n", total, chunks)
	// Output:
	// split 1048576 bytes into 5 chunks
}
