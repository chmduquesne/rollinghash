package bozo32_test

import (
	"fmt"
	"hash"
	"log"

	"github.com/chmduquesne/rollinghash/bozo32"
)

func Example() {
	s := []byte("The quick brown fox jumps over the lazy dog")

	classic := hash.Hash32(bozo32.New())
	rolling := bozo32.New()

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
			log.Fatalf("%v: expected %x, got %x",
				s[i-n+1:i+1], classic.Sum32(), rolling.Sum32())
		}
	}

	// Output:
	// he quick brown f: checksum 43ccedc8
	// e quick brown fo: checksum 58edb94f
	//  quick brown fox: checksum 24a53172
	// quick brown fox : checksum 2a953a52
	// uick brown fox j: checksum 68660e2b
	// ick brown fox ju: checksum a0dcc87b
	// ck brown fox jum: checksum a971cf
	// k brown fox jump: checksum 87384fec
	//  brown fox jumps: checksum 8aaa9434
	// brown fox jumps : checksum 930670f4
	// rown fox jumps o: checksum b1f3d3c1
	// own fox jumps ov: checksum 544099b5
	// wn fox jumps ove: checksum d4d1655b
	// n fox jumps over: checksum 1fafbea6
	//  fox jumps over : checksum cd48b1f8
	// fox jumps over t: checksum c986b2cc
	// ox jumps over th: checksum c6221c0e
	// x jumps over the: checksum aaf3c224
	//  jumps over the : checksum 316bd78c
	// jumps over the l: checksum 110b7f18
	// umps over the la: checksum 6580478f
	// mps over the laz: checksum 5b76ba4
	// ps over the lazy: checksum bedd0670
	// s over the lazy : checksum 43588f20
	//  over the lazy d: checksum cbaf2811
	// over the lazy do: checksum 579ec750
	// ver the lazy dog: checksum cfe7b948
}
