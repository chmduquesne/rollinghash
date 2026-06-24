package gearhash64_test

import (
	"fmt"
	"hash"
	"log"

	"github.com/chmduquesne/rollinghash/v4/gearhash64"
)

func Example() {
	s := []byte("The quick brown fox jumps over the lazy dog")

	classic := hash.Hash64(gearhash64.New())
	rolling := gearhash64.New()

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
	// he quick brown f: checksum 8266e99517d74e3f
	// e quick brown fo: checksum edf51911f895457f
	//  quick brown fox: checksum 7970b7aee863e103
	// quick brown fox : checksum 6d4a0ff7400acc05
	// uick brown fox j: checksum f79e9fe5c47882c6
	// ick brown fox ju: checksum 8ade39ee795c46f7
	// ck brown fox jum: checksum 65fc71b8c0153151
	// k brown fox jump: checksum da4680437cf3619d
	//  brown fox jumps: checksum aad11e02c261fcb7
	// brown fox jumps : checksum d00adc9ef407036d
	// rown fox jumps o: checksum b2f67616d6eaafdb
	// own fox jumps ov: checksum 2523af83d09e7243
	// wn fox jumps ove: checksum bb20ce1eb0ef377d
	// n fox jumps over: checksum 462850369ce05f25
	//  fox jumps over : checksum f90a53341766c849
	// fox jumps over t: checksum a3b8dce97926c756
	// ox jumps over th: checksum 6debbb66c6a673d6
	// x jumps over the: checksum 4cb0e5e49cff3aa3
	//  jumps over the : checksum ef679b745d3b7f45
	// jumps over the l: checksum 782668a4e3e6ab54
	// umps over the la: checksum 2a53d6ac06fd86b4
	// mps over the laz: checksum ac0b238f3ee227f8
	// ps over the lazy: checksum 80783d6ee6ef1204
	// s over the lazy : checksum 50753a5948252e07
	//  over the lazy d: checksum b6940bee550b4687
	// over the lazy do: checksum 7bafdc44e28360f
	// ver the lazy dog: checksum 1eb77581ae7093
}
