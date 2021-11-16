// Copyright (c) 2017 Christophe-Marie Duquesne <chmd@chmd.fr>

package rabinkarp64_test

import (
	"bufio"
	"crypto/rand"
	"hash"
	"io"
	"strings"
	"testing"

	"github.com/chmduquesne/rollinghash"
	rollsum "github.com/chmduquesne/rollinghash/rabinkarp64"
)

var golden = []struct {
	out uint64
	in  string
}{
	//{0x0, ""}, // panics
	{0x61, "a"},
	{0x6162, "ab"},
	{0x616263, "abc"},
	{0x61626364, "abcd"},
	{0x6162636465, "abcde"},
	{0x616263646566, "abcdef"},
	{0x132021ba359c68, "abcdefg"},
	{0x1fce6358ce1471, "abcdefgh"},
	{0x65b425e3c80ca, "abcdefghi"},
	{0xe9781880ddab2, "abcdefghij"},
	{0x1bcc435d5a6760, "Discard medicine more than two years old."},
	{0x1c56084394dbf5, "He who has a shady past knows that nice guys finish last."},
	{0x7973e4550080f, "I wouldn't marry him with a ten foot pole."},
	{0x1e2a9f14d4a366, "Free! Free!/A trip/to Mars/for 900/empty jars/Burma Shave"},
	{0x177a1e4d652838, "The days of the digital watch are numbered.  -Tom Stoppard"},
	{0x153bb8322d8614, "Nepal premier won't resign."},
	{0x12309044aaafcd, "For every action there is an equal and opposite government program."},
	{0x59187d7f34b99, "His money is twice tainted: 'taint yours and 'taint mine."},
	{0x5e4f5ec20dbb, "There is no reason for any individual to have a computer in their home. -Ken Olsen, 1977"},
	{0x5b605dca0167a, "It's a tiny change to the code and not completely disgusting. - Bob Manchek"},
	{0x1da35eec936b1c, "size:  a.out:  bad magic"},
	{0x1b4a521659269a, "The major problem is with sendmail.  -Mark Horton"},
	{0x11b3791cfaf6ef, "Give me a rock, paper and scissors and I will move the world.  CCFestoon"},
	{0x1ff6dcce41d7d9, "If the enemy is within range, then so are you."},
	{0x1820adc68f03ec, "It's well we cannot hear the screams/That we create in others' dreams."},
	{0x3f8660475a7fb, "You remind me of a TV show, but that's all right: I watch it anyway."},
	{0x149d09de60bc54, "C is as portable as Stonehedge!!"},
	{0x11686c8f59d7c7, "Even if I could be Shakespeare, I think I should still choose to be Faraday. - A. Huxley"},
	{0xfb8afb28c9bcf, "The fugacity of a constituent in a mixture of gases at a given temperature is proportional to its mole fraction.  Lewis-Randall Rule"},
	{0x1fa10a2313f3e6, "How can you write a big system without C++?  -Paul Glick"},
	{0x178cc568c9a3c, "'Invariant assertions' is the most elegant programming technique!  -Tom Szymanski"},
	{0x65bd2936a4628, strings.Repeat("\xff", 5548) + "8"},
	{0xe074cdecbffe1, strings.Repeat("\xff", 5549) + "9"},
	{0x1378f99580cada, strings.Repeat("\xff", 5550) + "0"},
	{0x1b6a3079f8c522, strings.Repeat("\xff", 5551) + "1"},
	{0x143e587f656d19, strings.Repeat("\xff", 5552) + "2"},
	{0xbadb5a7005edf, strings.Repeat("\xff", 5553) + "3"},
	{0xc040bc67bc471, strings.Repeat("\xff", 5554) + "4"},
	{0x1758803a1fc391, strings.Repeat("\xff", 5555) + "5"},
	{0x0, strings.Repeat("\x00", 1e5)},
	{0x4ded86a56a148, strings.Repeat("a", 1e5)},
	{0x2b16296a7e5a6, strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ", 1e4)},
}

// Prove that we implement various standard interfaces
var (
	_ hash.Hash64        = rollsum.New()
	_ rollinghash.Hash64 = rollsum.New()
	_ io.Writer          = rollsum.New()
)

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
	f := rand.Reader
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
