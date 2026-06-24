package rollinghash_test

import (
	"bytes"
	"fmt"
	"hash"
	"log"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	_adler32 "github.com/chmduquesne/rollinghash/v4/adler32"
	"github.com/chmduquesne/rollinghash/v4/bozo64"
)

func Example() {
	s := []byte("The quick brown fox jumps over the lazy dog")

	// This example works with adler32, but the api is identical for all
	// other rolling checksums. Consult the documentation of the checksum
	// of interest.
	classic := hash.Hash32(_adler32.New())
	rolling := _adler32.New()

	// Window len
	n := 16

	// You MUST load an initial window into the rolling hash before being
	// able to roll bytes
	if _, err := rolling.Write(s[:n]); err != nil {
		log.Fatal(err)
	}

	// Roll it and compare the result with full re-calculus every time
	for i := n; i < len(s); i++ {

		// Reset and write the window in classic
		classic.Reset()
		if _, err := classic.Write(s[i-n+1 : i+1]); err != nil {
			log.Fatal(err)
		}

		// Roll the incoming byte in rolling
		rolling.Roll(s[i])

		// Compare the hashes
		if classic.Sum32() != rolling.Sum32() {
			log.Fatalf("%v: expected %x, got %x",
				s[i-n+1:i+1], classic.Sum32(), rolling.Sum32())
		}
	}

}

// ExampleScanner shows how to use a Scanner to walk a stream and read, for
// every window position, the rolling checksum together with the bytes it
// covers. Here we locate a known block within the stream: the rolling
// checksum is the cheap filter, and the byte comparison confirms the match
// (rolling-hash matches can collide). The same loop shape serves chunking,
// analysis, or any custom rule over the checksums.
func ExampleScanner() {
	data := []byte("the quick brown fox jumps over the lazy dog")

	// The block we are looking for, and its rolling checksum.
	needle := []byte("brown")
	window := len(needle)

	h := bozo64.New()
	if _, err := h.Write(needle); err != nil {
		log.Fatal(err)
	}
	target := h.Sum64()

	// Scan the stream. Within each batch, Sums()[i] is the checksum of
	// Bytes()[i:i+window]. This input fits in a single batch, so the batch
	// index i is also the offset in the stream; for larger inputs spanning
	// multiple Scan() calls you would accumulate an offset across batches.
	s := rollinghash.NewScanner(bytes.NewReader(data), bozo64.New(), window)
	for s.Scan() {
		sums, buf := s.Sums(), s.Bytes()
		for i, sum := range sums {
			if sum == target && bytes.Equal(buf[i:i+window], needle) {
				fmt.Printf("found %q at offset %d\n", needle, i)
			}
		}
	}
	if err := s.Err(); err != nil {
		log.Fatal(err)
	}
	// Output: found "brown" at offset 10
}

// ExampleChunker demonstrates content-defined chunking: the stream is split
// where the rolling checksum hits a mask, with chunk sizes kept within
// [min, max]. The boundaries depend only on content, so they are stable under
// insertions and deletions elsewhere in the stream - the basis for
// deduplication.
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

	// Cut where the low 8 bits of the rolling checksum are zero, keeping each
	// chunk between 64 and 1024 bytes.
	c := rollinghash.NewChunker(bytes.NewReader(data), bozo64.New(), 32, 0xff, 64, 1024)

	var sizes []int
	total := 0
	for c.Next() {
		chunk := c.Chunk()
		sizes = append(sizes, len(chunk))
		total += len(chunk)
	}
	if err := c.Err(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("split %d bytes into %d chunks: %v\n", total, len(sizes), sizes)
	// Output: split 4096 bytes into 11 chunks: [354 82 381 661 549 255 764 308 145 344 253]
}
