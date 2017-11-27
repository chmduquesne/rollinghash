package buzhash32_test

import (
	"fmt"
	"hash"
	"log"

	"github.com/chmduquesne/rollinghash/buzhash32"
)

func Example() {
	s := []byte("The quick brown fox jumps over the lazy dog")

	classic := hash.Hash32(buzhash32.New())
	rolling := buzhash32.New()

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

		fmt.Printf("%v: checksum %x\n", string(s[i-n+1:i+1]), rolling.Sum32())

		// Compare the hashes
		if classic.Sum32() != rolling.Sum32() {
			log.Fatalf("%v: expected %s, got %s",
				s[i-n+1:i+1], classic.Sum32(), rolling.Sum32())
		}
	}

	// Output:
	// he quick brown f: checksum 53e7e066
	// e quick brown fo: checksum ecf5708c
	//  quick brown fox: checksum c12d0faf
	// quick brown fox : checksum f2e76fe2
	// uick brown fox j: checksum a8506342
	// ick brown fox ju: checksum 201db638
	// ck brown fox jum: checksum 759fe987
	// k brown fox jump: checksum ecf78a18
	//  brown fox jumps: checksum 9062a9c9
	// brown fox jumps : checksum 5078232e
	// rown fox jumps o: checksum b1d44d0d
	// own fox jumps ov: checksum 8177e796
	// wn fox jumps ove: checksum 135d33ca
	// n fox jumps over: checksum 7a45e290
	//  fox jumps over : checksum 1655abcb
	// fox jumps over t: checksum 710c1810
	// ox jumps over th: checksum bfb01cb9
	// x jumps over the: checksum 6ed2c594
	//  jumps over the : checksum f2e2c8e7
	// jumps over the l: checksum df544447
	// umps over the la: checksum 7df8d3c3
	// mps over the laz: checksum c8c88cc0
	// ps over the lazy: checksum 3e7f980c
	// s over the lazy : checksum fb4663b8
	//  over the lazy d: checksum 31ccb20e
	// over the lazy do: checksum c476b45f
	// ver the lazy dog: checksum afb3c2da
}
