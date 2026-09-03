package duplicacy_test

import (
	"bytes"
	"fmt"
	"log"

	duplicacy "github.com/chmduquesne/rollinghash/v4/cdc/compat/duplicacy"
)

// CreateChunkMaker mirrors Duplicacy's CreateFileChunkMaker: feed readers with
// AddData, then call it once more with a nil reader to flush the trailing
// bytes. With no options it uses the same seed and sizes as an unencrypted
// Duplicacy repository.
func ExampleChunkMaker() {
	data := make([]byte, 300*1024)
	x := uint32(1)
	for i := range data {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		data[i] = byte(x)
	}

	m := duplicacy.CreateChunkMaker(
		duplicacy.WithAverageChunkSize(8192),
		duplicacy.WithMinimumChunkSize(2048),
		duplicacy.WithMaximumChunkSize(65536),
	)

	total, chunks := 0, 0
	sink := func(c duplicacy.Chunk) {
		total += c.Length
		chunks++
	}
	if err := m.AddData(bytes.NewReader(data), sink); err != nil {
		log.Fatal(err)
	}
	if err := m.AddData(nil, sink); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("split %d bytes into %d chunks\n", total, chunks)
	// Output:
	// split 307200 bytes into 35 chunks
}
