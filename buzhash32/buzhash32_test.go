package buzhash32_test

import (
	"bufio"
	"crypto/rand"
	"hash"
	"io"
	"strings"
	"testing"

	"github.com/chmduquesne/rollinghash"
	rollsum "github.com/chmduquesne/rollinghash/buzhash32"
)

var golden = []struct {
	out uint32
	in  string
}{
	//{0x0, ""}, // panics
	{0x29ec300c, "a"},
	{0xf188df2c, "ab"},
	{0x44fce3ce, "abc"},
	{0x77c42de5, "abcd"},
	{0x573b093d, "abcde"},
	{0x67c55693, "abcdef"},
	{0x41d5a953, "abcdefg"},
	{0x9aeab78c, "abcdefgh"},
	{0x13411924, "abcdefghi"},
	{0x4dc2d8f4, "abcdefghij"},
	{0x99065f04, "Discard medicine more than two years old."},
	{0x5a6c6c9a, "He who has a shady past knows that nice guys finish last."},
	{0x51ac1bd0, "I wouldn't marry him with a ten foot pole."},
	{0x62268af0, "Free! Free!/A trip/to Mars/for 900/empty jars/Burma Shave"},
	{0x704eb745, "The days of the digital watch are numbered.  -Tom Stoppard"},
	{0xd4a23048, "Nepal premier won't resign."},
	{0x8eca545a, "For every action there is an equal and opposite government program."},
	{0x3b87b0da, "His money is twice tainted: 'taint yours and 'taint mine."},
	{0x4a1a9265, "There is no reason for any individual to have a computer in their home. -Ken Olsen, 1977"},
	{0xd3cc3586, "It's a tiny change to the code and not completely disgusting. - Bob Manchek"},
	{0xd6ce8c5a, "size:  a.out:  bad magic"},
	{0x47eaad99, "The major problem is with sendmail.  -Mark Horton"},
	{0x5ec2ffab, "Give me a rock, paper and scissors and I will move the world.  CCFestoon"},
	{0x3a34b15, "If the enemy is within range, then so are you."},
	{0x532df3e8, "It's well we cannot hear the screams/That we create in others' dreams."},
	{0x7cbcf246, "You remind me of a TV show, but that's all right: I watch it anyway."},
	{0xa653a57f, "C is as portable as Stonehedge!!"},
	{0xaa4a402c, "Even if I could be Shakespeare, I think I should still choose to be Faraday. - A. Huxley"},
	{0xbc881e9c, "The fugacity of a constituent in a mixture of gases at a given temperature is proportional to its mole fraction.  Lewis-Randall Rule"},
	{0xfaffeaf5, "How can you write a big system without C++?  -Paul Glick"},
	{0x3775c6ce, "'Invariant assertions' is the most elegant programming technique!  -Tom Szymanski"},
	{0x92d904de, strings.Repeat("\xff", 5548) + "8"},
	{0x280326bc, strings.Repeat("\xff", 5549) + "9"},
	{0xc8c4ec97, strings.Repeat("\xff", 5550) + "0"},
	{0x67e7441d, strings.Repeat("\xff", 5551) + "1"},
	{0x95601eb, strings.Repeat("\xff", 5552) + "2"},
	{0x37ce09c7, strings.Repeat("\xff", 5553) + "3"},
	{0x64126a4b, strings.Repeat("\xff", 5554) + "4"},
	{0x7a492c8e, strings.Repeat("\xff", 5555) + "5"},
	{0xffffffff, strings.Repeat("\x00", 1e5)},
	{0x0, strings.Repeat("a", 1e5)},
	{0x55555555, strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ", 1e4)},
}

// Prove that we implement various standard interfaces
var (
	_ hash.Hash32        = rollsum.New()
	_ rollinghash.Hash32 = rollsum.New()
	_ io.Writer          = rollsum.New()
)

// Sum32ByWriteAndRoll computes the sum by prepending the input slice with
// a '\0', writing the first bytes of this slice into the sum, then
// sliding on the last byte and returning the result of Sum32
func Sum32ByWriteAndRoll(b []byte) uint32 {
	q := []byte("\x00")
	q = append(q, b...)
	roll := rollsum.New()
	if _, err := roll.Write(q[:len(q)-1]); err != nil {
		panic(err)
	}
	roll.Roll(q[len(q)-1])
	return roll.Sum32()
}

func TestGolden(t *testing.T) {
	for _, g := range golden {
		in := g.in

		// We test the classic implementation
		p := []byte(g.in)
		classic := hash.Hash32(rollsum.New())
		if _, err := classic.Write(p); err != nil {
			t.Fatal(err)
		}
		if got := classic.Sum32(); got != g.out {
			t.Errorf("classic implentation: for %q, expected 0x%x, got 0x%x", in, g.out, got)
			continue
		}

		if got := Sum32ByWriteAndRoll(p); got != g.out {
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
	if _, err := h.Write(window); err != nil {
		b.Fatal(err)
	}

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
	if _, err := h.Write(window); err != nil {
		b.Fatal(err)
	}

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
