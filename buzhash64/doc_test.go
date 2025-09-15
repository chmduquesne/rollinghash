package buzhash64_test

import (
	"fmt"
	"hash"
	"log"

	"github.com/chmduquesne/rollinghash/buzhash64"
)

func Example() {
	s := []byte("The quick brown fox jumps over the lazy dog")

	classic := hash.Hash64(buzhash64.New())
	rolling := buzhash64.New()

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

		fmt.Printf("%v: checksum %x\n", string(s[i-n+1:i+1]), rolling.Sum64())

		// Compare the hashes
		if classic.Sum64() != rolling.Sum64() {
			log.Fatalf("%v: expected %x, got %x",
				s[i-n+1:i+1], classic.Sum64(), rolling.Sum64())
		}
	}

	// Output:
	// he quick brown f: checksum 27b900ac53e7f3ee
	// e quick brown fo: checksum b05b4730ecf51388
	//  quick brown fox: checksum 473f08ecc12d2117
	// quick brown fox : checksum 83e17140f2e75f95
	// uick brown fox j: checksum 64d46288a85055a9
	// ick brown fox ju: checksum a5cbdf3e201dada3
	// ck brown fox jum: checksum 9b57bc98759f926a
	// k brown fox jump: checksum 401d9a62ecf7eeab
	//  brown fox jumps: checksum 8bf712419062ba1c
	// brown fox jumps : checksum 1a71441a50786982
	// rown fox jumps o: checksum a101754db1d45e07
	// own fox jumps ov: checksum 375b7cc8177aedf
	// wn fox jumps ove: checksum 9c1dcae135d3b27
	// n fox jumps over: checksum a3a8e55b7a45f607
	//  fox jumps over : checksum 749fb4791655a8bf
	// fox jumps over t: checksum bb9bbe73710c73fe
	// ox jumps over th: checksum 1cb97e12bfb044bc
	// x jumps over the: checksum 36584f126ed2efe1
	//  jumps over the : checksum 46cacdcff2e2ec93
	// jumps over the l: checksum a77c4823df5461a8
	// umps over the la: checksum 88f38ba47df8f34d
	// mps over the laz: checksum 39428e93c8c8bb91
	// ps over the lazy: checksum 1a2767543e7f8a8c
	// s over the lazy : checksum 64b58d2cfb461f2b
	//  over the lazy d: checksum 5cc1b31e31cca116
	// over the lazy do: checksum 94364057c476ff69
	// ver the lazy dog: checksum 38974742afb3cec8
}
