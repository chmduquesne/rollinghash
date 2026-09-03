package buildbarn_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"

	cdc "github.com/chmduquesne/rollinghash/v4/cdc/compat/buildbarn"
)

// cdc.NewMaxContentDefinedChunker + NewChunkReader + ReadNextChunk-until-io.EOF
// mirror github.com/buildbarn/go-cdc. Migrating from that package is just
// changing the import path.
func ExampleNewMaxContentDefinedChunker() {
	// Repeatable pseudo-random data (xorshift), so boundaries are stable.
	data := make([]byte, 64*1024)
	x := uint32(1)
	for i := range data {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		data[i] = byte(x)
	}

	chunker := cdc.NewMaxContentDefinedChunker(&cdc.FastContentDefinedChunkerGearTable, 4<<10, 16<<10)
	r := chunker.NewChunkReader(bufio.NewReaderSize(bytes.NewReader(data), 64<<10))

	total, chunks := 0, 0
	for {
		chunk, err := r.ReadNextChunk()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		total += len(chunk)
		chunks++
	}
	fmt.Printf("split %d bytes into %d chunks\n", total, chunks)
	// Output:
	// split 65536 bytes into 7 chunks
}
