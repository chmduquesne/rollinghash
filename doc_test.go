package rollinghash_test

import (
	"hash"
	"log"

	_adler32 "github.com/chmduquesne/rollinghash/adler32"
)

func Example() {
	s := []byte("The quick brown fox jumps over the lazy dog")

	// This example works with adler32, but the api is identical for all
	// other rolling checksums. Consult the documentation of the checksum
	// you are interested in to find similar examples.
	classic := hash.Hash32(_adler32.New())
	rolling := _adler32.New()

	// Window len
	n := 16

	// You MUST load an initial window into the rolling hash before being
	// able to roll bytes
	rolling.Write(s[:n])

	// Roll it and compare the result with full re-calculus every time
	for i := n; i < len(s); i++ {

		// Reset and write the window in classic
		classic.Reset()
		classic.Write(s[i-n+1 : i+1])

		// Roll the incoming byte in rolling
		rolling.Roll(s[i])

		// Compare the hashes
		if classic.Sum32() != rolling.Sum32() {
			log.Fatalf("%v: expected %s, got %s",
				s[i-n+1:i+1], classic.Sum32(), rolling.Sum32())
		}
	}

}
