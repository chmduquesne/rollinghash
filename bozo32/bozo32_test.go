package bozo32_test

import (
	"bufio"
	"hash"
	"io"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/chmduquesne/rollinghash"
	rollsum "github.com/chmduquesne/rollinghash/bozo32"
)

var golden = []struct {
	out uint32
	in  string
}{
	//{0x0, ""}, // panics
	{0x61, "a"},
	{0x60fab3, "ab"},
	{0xf5044fe6, "abc"},
	{0xf4a551ea, "abcd"},
	{0xfc3a33af, "abcde"},
	{0x6c45f925, "abcdef"},
	{0xa10b673c, "abcdefg"},
	{0xf790f3e4, "abcdefgh"},
	{0x7265b60d, "abcdefghi"},
	{0x21755a7, "abcdefghij"},
	{0xc0e67701, "Discard medicine more than two years old."},
	{0xb62ddac6, "He who has a shady past knows that nice guys finish last."},
	{0x29941b90, "I wouldn't marry him with a ten foot pole."},
	{0xbdfa9c64, "Free! Free!/A trip/to Mars/for 900/empty jars/Burma Shave"},
	{0x3973640f, "The days of the digital watch are numbered.  -Tom Stoppard"},
	{0x2caf4c69, "Nepal premier won't resign."},
	{0x4370a2fc, "For every action there is an equal and opposite government program."},
	{0x105c181, "His money is twice tainted: 'taint yours and 'taint mine."},
	{0xf636f6a2, "There is no reason for any individual to have a computer in their home. -Ken Olsen, 1977"},
	{0xd53ee79, "It's a tiny change to the code and not completely disgusting. - Bob Manchek"},
	{0xfa8b9ee, "size:  a.out:  bad magic"},
	{0xbf79f440, "The major problem is with sendmail.  -Mark Horton"},
	{0x9da762a3, "Give me a rock, paper and scissors and I will move the world.  CCFestoon"},
	{0x658aa63a, "If the enemy is within range, then so are you."},
	{0xfa49ca46, "It's well we cannot hear the screams/That we create in others' dreams."},
	{0x419a8bb6, "You remind me of a TV show, but that's all right: I watch it anyway."},
	{0x1d9b58d8, "C is as portable as Stonehedge!!"},
	{0x9234f2df, "Even if I could be Shakespeare, I think I should still choose to be Faraday. - A. Huxley"},
	{0x7e43d6de, "The fugacity of a constituent in a mixture of gases at a given temperature is proportional to its mole fraction.  Lewis-Randall Rule"},
	{0xf2f16f5, "How can you write a big system without C++?  -Paul Glick"},
	{0xd9e43015, "'Invariant assertions' is the most elegant programming technique!  -Tom Szymanski"},
	{0x28ffc26c, strings.Repeat("\xff", 5548) + "8"},
	{0x5c36903c, strings.Repeat("\xff", 5549) + "9"},
	{0x29cf8112, strings.Repeat("\xff", 5550) + "0"},
	{0xeb86402, strings.Repeat("\xff", 5551) + "1"},
	{0x88021802, strings.Repeat("\xff", 5552) + "2"},
	{0x20af8c12, strings.Repeat("\xff", 5553) + "3"},
	{0xa294bf32, strings.Repeat("\xff", 5554) + "4"},
	{0x3945c062, strings.Repeat("\xff", 5555) + "5"},
	{0x0, strings.Repeat("\x00", 1e5)},
	{0xc18a57a0, strings.Repeat("a", 1e5)},
	{0xc7d4c4f0, strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ", 1e4)},
}

// Prove that we implement various standard interfaces
var (
	_ hash.Hash32        = rollsum.New()
	_ rollinghash.Hash32 = rollsum.New()
	_ io.Writer          = rollsum.New()
	_ io.Reader          = rollsum.New()
)

// Sum32ByWriteAndRoll computes the sum by prepending the input slice with
// a '\0', writing the first bytes of this slice into the sum, then
// sliding on the last byte and returning the result of Sum32
func Sum32ByWriteAndRoll(b []byte) uint32 {
	q := []byte("\x00")
	q = append(q, b...)
	roll := rollsum.New()
	roll.Write(q[:len(q)-1])
	roll.Roll(q[len(q)-1])
	return roll.Sum32()
}

func TestGolden(t *testing.T) {
	for _, g := range golden {
		in := g.in

		// We test the classic implementation
		p := []byte(g.in)
		classic := hash.Hash32(rollsum.New())
		classic.Write(p)
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
