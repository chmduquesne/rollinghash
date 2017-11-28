package rabinkarp64_test

import (
	"fmt"
	"hash"
	"log"

	"github.com/chmduquesne/rollinghash/rabinkarp64"
)

func Example() {
	s := []byte("The quick brown fox jumps over the lazy dog")

	classic := hash.Hash64(rabinkarp64.New())
	rolling := rabinkarp64.New()

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

		fmt.Printf("%v: checksum %x\n", string(s[i-n+1:i+1]), rolling.Sum64())

		// Compare the hashes
		if classic.Sum64() != rolling.Sum64() {
			log.Fatalf("%v: expected %x, got %x",
				string(s[i-n+1:i+1]), classic.Sum64(), rolling.Sum64())
		}
	}

	// Output:

	// he quick brown f: checksum 1ab89e68de7c15
	// e quick brown fo: checksum 1d26864e21619f
	//  quick brown fox: checksum 13fdc4aaefcf91
	// quick brown fox : checksum 1fab0ef7daee4a
	// uick brown fox j: checksum 6aee0bda40445
	// ick brown fox ju: checksum a8cf05560301d
	// ck brown fox jum: checksum 1945eaabdb6b67
	// k brown fox jump: checksum 18964a2ca37033
	//  brown fox jumps: checksum 7f1778d0e6456
	// brown fox jumps : checksum 8d3dd9e2cf5a3
	// rown fox jumps o: checksum 5e7672798a4e5
	// own fox jumps ov: checksum 41e75561bd7ce
	// wn fox jumps ove: checksum 1db9e271edcead
	// n fox jumps over: checksum 7aeec087fe22a
	//  fox jumps over : checksum 1a3acd0bfd0c1f
	// fox jumps over t: checksum c3620e2c8d91a
	// ox jumps over th: checksum 8b7049026154d
	// x jumps over the: checksum 1f639b25356c1d
	//  jumps over the : checksum a7961a2d0f9c4
	// jumps over the l: checksum 6e3c3ec495a7d
	// umps over the la: checksum a3dbdf68d695e
	// mps over the laz: checksum 1c443a5f275ca7
	// ps over the lazy: checksum 57e965da5efe2
	// s over the lazy : checksum 1d457b44849f9d
	//  over the lazy d: checksum 1d54040df5f20f
	// over the lazy do: checksum 1aa7779b59c5fb
	// ver the lazy dog: checksum 1d72c7f255ba24
}
