package adler32_test

import (
	"fmt"
	"hash"
	"hash/adler32"
	"log"

	_adler32 "github.com/chmduquesne/rollinghash/adler32"
)

func Example() {
	s := []byte("The quick brown fox jumps over the lazy dog")

	classic := hash.Hash32(adler32.New())
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

		fmt.Printf("%v: checksum %x\n", string(s[i-n+1:i+1]), rolling.Sum32())

		// Compare the hashes
		if classic.Sum32() != rolling.Sum32() {
			log.Fatalf("%v: expected %x, got %x",
				s[i-n+1:i+1], classic.Sum32(), rolling.Sum32())
		}
	}

	// Output:
	// he quick brown f: checksum 31e905d9
	// e quick brown fo: checksum 314805e0
	//  quick brown fox: checksum 30ea05f3
	// quick brown fox : checksum 34dc05f3
	// uick brown fox j: checksum 33b705ec
	// ick brown fox ju: checksum 325205ec
	// ck brown fox jum: checksum 31b105f0
	// k brown fox jump: checksum 317d05fd
	//  brown fox jumps: checksum 30d10605
	// brown fox jumps : checksum 34d50605
	// rown fox jumps o: checksum 34c60612
	// own fox jumps ov: checksum 33bb0616
	// wn fox jumps ove: checksum 32d6060c
	// n fox jumps over: checksum 316c0607
	//  fox jumps over : checksum 304405b9
	// fox jumps over t: checksum 3450060d
	// ox jumps over th: checksum 33fe060f
	// x jumps over the: checksum 33120605
	//  jumps over the : checksum 313e05ad
	// jumps over the l: checksum 353605f9
	// umps over the la: checksum 348505f0
	// mps over the laz: checksum 332905f5
	// ps over the lazy: checksum 32590601
	// s over the lazy : checksum 310905b1
	//  over the lazy d: checksum 2f7a05a2
	// over the lazy do: checksum 336a05f1
	// ver the lazy dog: checksum 326205e9
}
