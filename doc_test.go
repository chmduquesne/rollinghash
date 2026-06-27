package rollinghash_test

import (
	"bytes"
	"fmt"
	"hash"
	"log"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	_adler32 "github.com/chmduquesne/rollinghash/v4/adler32"
	"github.com/chmduquesne/rollinghash/v4/buzhash64"
)

// Using Roll() is the easiest way to use this library. Because it manages
// an internal rolling window, it is very user-friendly. Unfortunately
// this user-friendliness costs CPU cycles. Consider using the Scanner or
// the Chunker interface if you want the highest speed.
func ExampleHash_Roll() {
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

// The Scanner interface was designed to support users who want to search
// for a given block within a stream, rsync-style. In this type of
// situation, the rolling checksum would be used as a cheap filter, and
// another method (e.g. byte comparison) confirms the match. This
// interface is shaped like a bufio.Scanner. Because it can batch the
// computations, it can use optimization techniques such as ILP
// exploitation to parallelize processing. This results into a performance
// that is almost doubled compared to Roll().
func ExampleScanner() {
	data := []byte("the quick brown fox jumps over the lazy dog")

	// The block we are looking for, and its rolling checksum.
	needle := []byte("brown")
	window := len(needle)

	h := buzhash64.New()
	if _, err := h.Write(needle); err != nil {
		log.Fatal(err)
	}
	target := h.Sum64()

	// Scan the stream. Within each batch, Sums()[i] is the checksum of
	// Bytes()[i:i+window]. This input fits in a single batch, so the batch
	// index i is also the offset in the stream; for larger inputs spanning
	// multiple Scan() calls you would accumulate an offset across batches.
	s := rollinghash.NewScanner(bytes.NewReader(data), buzhash64.New(), window)
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

// Buffer controls the batch size used by the Scanner. A larger buffer means
// fewer Scan() calls and better amortization of the bulk fast path, at the
// cost of higher memory use. The default buffer is 64 KiB; here we use a
// small buffer to show that the Scanner produces correct results regardless
// of how the input is split across batches.
func ExampleScanner_Buffer() {
	data := []byte("the quick brown fox jumps over the lazy dog")

	needle := []byte("brown")
	window := len(needle)

	h := buzhash64.New()
	if _, err := h.Write(needle); err != nil {
		log.Fatal(err)
	}
	target := h.Sum64()

	s := rollinghash.NewScanner(bytes.NewReader(data), buzhash64.New(), window)
	// Use the smallest valid buffer (window bytes) so every Scan() call
	// returns exactly one position, exercising the batch-boundary logic.
	s.Buffer(make([]byte, window))

	off := 0
	for s.Scan() {
		sums, buf := s.Sums(), s.Bytes()
		for i, sum := range sums {
			if sum == target && bytes.Equal(buf[i:i+window], needle) {
				fmt.Printf("found %q at offset %d\n", needle, off+i)
			}
		}
		off += len(buf) - (window - 1)
	}
	if err := s.Err(); err != nil {
		log.Fatal(err)
	}
	// Output: found "brown" at offset 10
}

// Reset lets you reuse a Scanner's internal buffers for a new stream without
// any extra allocations. This matters when scanning many streams in a loop.
func ExampleScanner_Reset() {
	needle := []byte("fox")
	window := len(needle)

	h := buzhash64.New()
	if _, err := h.Write(needle); err != nil {
		log.Fatal(err)
	}
	target := h.Sum64()

	streams := [][]byte{
		[]byte("the quick brown fox jumps over the lazy dog"),
		[]byte("a fox and another fox"),
	}

	s := rollinghash.NewScanner(nil, buzhash64.New(), window)
	for i, data := range streams {
		s.Reset(bytes.NewReader(data))
		count := 0
		for s.Scan() {
			for _, sum := range s.Sums() {
				if sum == target {
					count++
				}
			}
		}
		if err := s.Err(); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("stream %d: %d match(es)\n", i, count)
	}
	// Output:
	// stream 0: 1 match(es)
	// stream 1: 2 match(es)
}

// The Chunker interface was designed to support users who want to use
// rolling hashes for Content Defined Chunking (CDC). It also operates on
// a stream, which allows for batch computation optimizations similar to
// the ones used with the Scanner. In this type of situation, The stream
// is split where the rolling checksum hits a mask, with chunk sizes kept
// within [min, max].
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
	c := rollinghash.NewChunker(bytes.NewReader(data), buzhash64.New(), 56, 0xff, 64, 1024)

	var sizes []int
	total := 0
	for c.Next() {
		chunk := c.Bytes()
		sizes = append(sizes, len(chunk))
		total += len(chunk)
		if c.AtMask() {
			fmt.Printf("boundary at %d: sum=0x%x\n", total, c.Sum())
		} else {
			fmt.Printf("max cut   at %d\n", total)
		}
	}
	if err := c.Err(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("split %d bytes into %d chunks: %v\n", total, len(sizes), sizes)
	// Output:
	// boundary at 123: sum=0xbd9f33d05f52e700
	// boundary at 277: sum=0x3a611bc3e53cf900
	// boundary at 651: sum=0xecb29647a13a3600
	// boundary at 769: sum=0x3109e14cbfa7da00
	// boundary at 1436: sum=0xb61cdeda53dac000
	// boundary at 1522: sum=0x8bef143657fed400
	// boundary at 1722: sum=0x35f525a03a01d000
	// boundary at 2173: sum=0xb168bf8f4418ee00
	// boundary at 2404: sum=0x6c13f4fb45436f00
	// boundary at 2647: sum=0x33695e700dcdf300
	// boundary at 3388: sum=0xe915cd64f38a9800
	// boundary at 3837: sum=0xdfc83351b3d06800
	// max cut   at 4096
	// split 4096 bytes into 13 chunks: [123 154 374 118 667 86 200 451 231 243 741 449 259]
}

// Reset lets you reuse a Chunker's internal buffers for a new stream without
// any extra allocations. This matters when chunking many streams in a loop.
func ExampleChunker_Reset() {
	makeData := func(seed uint32, n int) []byte {
		data := make([]byte, n)
		x := seed
		for i := range data {
			x ^= x << 13
			x ^= x >> 17
			x ^= x << 5
			data[i] = byte(x)
		}
		return data
	}

	streams := [][]byte{
		makeData(1, 4096),
		makeData(2, 4096),
	}

	c := rollinghash.NewChunker(nil, buzhash64.New(), 56, 0xff, 64, 1024)
	for i, data := range streams {
		c.Reset(bytes.NewReader(data))
		n := 0
		for c.Next() {
			n++
		}
		if err := c.Err(); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("stream %d: %d chunks\n", i, n)
	}
	// Output:
	// stream 0: 13 chunks
	// stream 1: 14 chunks
}
