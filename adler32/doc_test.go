package adler32_test

import (
	"hash/adler32"
	"log"

	rollsum "github.com/chmduquesne/rollinghash/adler32"
)

var data = "The quick brown fox jumps over the lazy dog"

func Example() {
	s := []byte(data)

	vanilla := adler32.New()
	rolling := rollsum.New()

	// arbitrary window len
	n := 16

	// Load the window into the rolling hash
	rolling.Write(s[:n])

	// Roll it and compare the result with full re-calculus every time
	for i := n; i < len(s); i++ {

		vanilla.Reset()
		vanilla.Write(s[i-n+1 : i+1])

		err := rolling.Roll(s[i])
		if err != nil {
			log.Fatal(err)
		}

		if vanilla.Sum32() != rolling.Sum32() {
			log.Fatalf("%v: expected %x, got %x",
				s[i-n+1:i+1], vanilla.Sum32(), rolling.Sum32())
		}

	}
}
