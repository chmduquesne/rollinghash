package buzhash64_test

import (
	"bufio"
	"hash"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/chmduquesne/rollinghash"
	rollsum "github.com/chmduquesne/rollinghash/buzhash64"
)

var golden = []struct {
	out uint64
	in  string
}{
	{0x0, ""},
	{0x103970a329ec300c, "a"},
	{0x47040cbf188df2c, "ab"},
	{0x3c65f4e944fce3cf, "abc"},
	{0x4f60df9377c42de7, "abcd"},
	{0xcec2a201573b0939, "abcde"},
	{0xdbee72d967c5569a, "abcdef"},
	{0xf80e8c4f41d5a940, "abcdefg"},
	{0xad48d3d99aeab7ab, "abcdefgh"},
	{0x30df48c91341196a, "abcdefghi"},
	{0x61ae47a04dc2d868, "abcdefghij"},
	{0x865ce00093c2d75a, "Discard medicine more than two years old."},
	{0xf6451e42f92f4791, "He who has a shady past knows that nice guys finish last."},
	{0xffda8ce848bb89aa, "I wouldn't marry him with a ten foot pole."},
	{0x5c12e27de6d83a30, "Free! Free!/A trip/to Mars/for 900/empty jars/Burma Shave"},
	{0x49e0ad4ab0ca7ce5, "The days of the digital watch are numbered.  -Tom Stoppard"},
	{0xd89090cdd4dd4903, "Nepal premier won't resign."},
	{0x22e4214eed90dea, "For every action there is an equal and opposite government program."},
	{0x314d09d305b82be6, "His money is twice tainted: 'taint yours and 'taint mine."},
	{0x2e08403f31426a4f, "There is no reason for any individual to have a computer in their home. -Ken Olsen, 1977"},
	{0x2f1b953fa66c1447, "It's a tiny change to the code and not completely disgusting. - Bob Manchek"},
	{0xb5f6b055d6aadc23, "size:  a.out:  bad magic"},
	{0x5859ccebfdaddec6, "The major problem is with sendmail.  -Mark Horton"},
	{0x2576ff98789ac55a, "Give me a rock, paper and scissors and I will move the world.  CCFestoon"},
	{0xc4022ec5788ce718, "If the enemy is within range, then so are you."},
	{0x68c85e74ecf463e4, "It's well we cannot hear the screams/That we create in others' dreams."},
	{0x315778aa0cbc1b2, "You remind me of a TV show, but that's all right: I watch it anyway."},
	{0x44b49393f2b0a0de, "C is as portable as Stonehedge!!"},
	{0xc81fb312ed79c064, "Even if I could be Shakespeare, I think I should still choose to be Faraday. - A. Huxley"},
	{0xc732298d7f79e413, "The fugacity of a constituent in a mixture of gases at a given temperature is proportional to its mole fraction.  Lewis-Randall Rule"},
	{0x708bba6595ed094a, "How can you write a big system without C++?  -Paul Glick"},
	{0x79f053c321ef8b17, "'Invariant assertions' is the most elegant programming technique!  -Tom Szymanski"},
	{0x1048f99c5be4db21, strings.Repeat("\xff", 5548) + "8"},
	{0x1a5a34feba789943, strings.Repeat("\xff", 5549) + "9"},
	{0xab1feddbec339368, strings.Repeat("\xff", 5550) + "0"},
	{0xe493ddbd2e09bbe2, strings.Repeat("\xff", 5551) + "1"},
	{0xb00cf0719a8bfe14, strings.Repeat("\xff", 5552) + "2"},
	{0xf274c02c1075f638, strings.Repeat("\xff", 5553) + "3"},
	{0x8f9ddd932b6595b4, strings.Repeat("\xff", 5554) + "4"},
	{0xa2b79cf7e4a6d371, strings.Repeat("\xff", 5555) + "5"},
	{0x3988d52ec6772ad1, strings.Repeat("\x00", 1e5)},
	{0x174cc065174cc065, strings.Repeat("a", 1e5)},
	{0x598883fc0cddd6a9, strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ", 1e4)},
}

// Prove that we implement rollinghash.Hash64
var _ = rollinghash.Hash64(rollsum.New())

// Prove that we implement hash.Hash64
var _ = hash.Hash64(rollsum.New())

// Sum64ByWriteAndRoll computes the sum by prepending the input slice with
// a '\0', writing the first bytes of this slice into the sum, then
// sliding on the last byte and returning the result of Sum32
func Sum64ByWriteAndRoll(b []byte) uint64 {
	q := []byte("\x00")
	q = append(q, b...)
	roll := rollsum.New()
	roll.Write(q[:len(q)-1])
	roll.Roll(q[len(q)-1])
	return roll.Sum64()
}

func TestGolden(t *testing.T) {
	for _, g := range golden {
		in := g.in

		// We test the classic implementation
		p := []byte(g.in)
		classic := hash.Hash64(rollsum.New())
		classic.Write(p)
		if got := classic.Sum64(); got != g.out {
			t.Errorf("classic implentation: for %q, expected 0x%x, got 0x%x", in, g.out, got)
			continue
		}

		if got := Sum64ByWriteAndRoll(p); got != g.out {
			t.Errorf("rolling implentation: for %q, expected 0x%x, got 0x%x", in, g.out, got)
			continue
		}
	}
}

func BenchmarkRolling64B(b *testing.B) {
	b.SetBytes(1024)
	b.ReportAllocs()
	window := make([]byte, 64)
	for i := range window {
		window[i] = byte(i)
	}

	h := rollsum.New()
	in := make([]byte, 0, h.Size())
	h.Write(window)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Roll(byte(i))
		h.Sum(in)
	}
}

func BenchmarkReadUrandom(b *testing.B) {
	b.SetBytes(1024)
	b.ReportAllocs()
	f, err := os.Open("/dev/urandom")
	if err != nil {
		b.Errorf("Could not open /dev/urandom")
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}()
	r := bufio.NewReader(f)
	ws := 64
	window := make([]byte, ws)
	n, err := r.Read(window)
	if n != ws || err != nil {
		b.Errorf("Could not read %d bytes", ws)
	}

	h := rollsum.New()
	in := make([]byte, 0, h.Size())
	h.Write(window)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c, err := r.ReadByte()
		if err != nil {
			b.Errorf("%s", err)
		}
		h.Roll(c)
		h.Sum(in)
	}
}
