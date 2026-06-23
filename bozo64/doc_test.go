package bozo64_test

import (
	"fmt"
	"hash"
	"log"

	"github.com/chmduquesne/rollinghash/v4/bozo64"
)

func Example() {
	s := []byte("The quick brown fox jumps over the lazy dog")

	classic := hash.Hash64(bozo64.New())
	rolling := bozo64.New()

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
	// he quick brown f: checksum c18b2e59c43de682
	// e quick brown fo: checksum 6ae404d2584d197d
	//  quick brown fox: checksum a0b93d6d08d869e2
	// quick brown fox : checksum dfcd4c4ff577f696
	// uick brown fox j: checksum 334cdd27a3a4db4b
	// ick brown fox ju: checksum ff4e505820faa4c9
	// ck brown fox jum: checksum 42a5b1ce01aaf257
	// k brown fox jump: checksum b3a58403c7e80d1a
	//  brown fox jumps: checksum c388491db1230946
	// brown fox jumps : checksum da0cb140ac02d9a2
	// rown fox jumps o: checksum b925d564fb22f863
	// own fox jumps ov: checksum aa11ca38005b6295
	// wn fox jumps ove: checksum eb30a2457b189ecd
	// n fox jumps over: checksum 9ef86b9dddd2f3ba
	//  fox jumps over : checksum 505273c0aeb53890
	// fox jumps over t: checksum 17ac0b5bb827ed84
	// ox jumps over th: checksum 50808971a29fd5ee
	// x jumps over the: checksum 4d4b597e4fc25e10
	//  jumps over the : checksum 38f12f712f8fc758
	// jumps over the l: checksum d6cefb133e32394
	// umps over the la: checksum bec9ac881c2e0893
	// mps over the laz: checksum be6770bdc64cc266
	// ps over the lazy: checksum ebcc9020ab469f4e
	// s over the lazy : checksum fd4a9a1f9d8dff2a
	//  over the lazy d: checksum a79992064d51d0df
	// over the lazy do: checksum 1e50c4e9f18f3f4
	// ver the lazy dog: checksum d2cde9336164c7f4
}
