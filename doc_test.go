package rollinghash_test

import (
	"log"

	_adler32 "github.com/chmduquesne/rollinghash/adler32"
)

func Example() {
	s := []byte("The quick brown fox jumps over the lazy dog")

	// You can substitute _adler32 for any other subpackage
	classic := _adler32.New()
	rolling := _adler32.New()

	// Window len
	n := 16

	// Load the window into the rolling hash
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
