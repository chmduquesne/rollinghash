package gearhash64_test

import (
	"bufio"
	"hash"
	"io"
	"math/rand"
	"strings"
	"testing"

	"github.com/chmduquesne/rollinghash/v4"
	rollsum "github.com/chmduquesne/rollinghash/v4/gearhash64"
)

var golden = []struct {
	out uint64
	in  string
}{
	{0x103970a329ec300c, "a"},
	{0xc47582d3f6291f4c, "ab"},
	{0xbd707b26943f9c2f, "abc"},
	{0x328c2c8f26bd22d7, "abcd"},
	{0x351b7646062d98a5, "abcde"},
	{0x30a22367d60e7633, "abcdef"},
	{0xb116b0cd3a7bf0db, "abcdefg"},
	{0xbf832ce18e39c6e0, "abcdefgh"},
	{0x6955493d430803fd, "abcdefghi"},
	{0x52bb68acf150f2b6, "abcdefghij"},
	{0x7ffb56dde3d75ee1, "Discard medicine more than two years old."},
	{0x1b6e670e21f605cb, "He who has a shady past knows that nice guys finish last."},
	{0xf89b5507a5a4bc9d, "I wouldn't marry him with a ten foot pole."},
	{0xe72a3e40bf902dc1, "Free! Free!/A trip/to Mars/for 900/empty jars/Burma Shave"},
	{0x4cc88450c6e96b27, "The days of the digital watch are numbered.  -Tom Stoppard"},
	{0x4f66deb2d3d2ab53, "Nepal premier won't resign."},
	{0xc6cb7d9b3f1f726d, "For every action there is an equal and opposite government program."},
	{0x511ca08d48c0abc5, "His money is twice tainted: 'taint yours and 'taint mine."},
	{0xa6c04ba8df3920bd, "There is no reason for any individual to have a computer in their home. -Ken Olsen, 1977"},
	{0xbc099477d741b7d9, "It's a tiny change to the code and not completely disgusting. - Bob Manchek"},
	{0x959748cc25b1d495, "size:  a.out:  bad magic"},
	{0xe256d97a5955c056, "The major problem is with sendmail.  -Mark Horton"},
	{0x9aea055ffa36d532, "Give me a rock, paper and scissors and I will move the world.  CCFestoon"},
	{0xa2730464d948af79, "If the enemy is within range, then so are you."},
	{0xfa35aa7802e4f0e5, "It's well we cannot hear the screams/That we create in others' dreams."},
	{0xf40c9fa34c00d0ff, "You remind me of a TV show, but that's all right: I watch it anyway."},
	{0xd3643ea4058de9b5, "C is as portable as Stonehedge!!"},
	{0xcc8de1e97e8a3a42, "Even if I could be Shakespeare, I think I should still choose to be Faraday. - A. Huxley"},
	{0xfea391e4322e3f07, "The fugacity of a constituent in a mixture of gases at a given temperature is proportional to its mole fraction.  Lewis-Randall Rule"},
	{0xfe5884cb7217740d, "How can you write a big system without C++?  -Paul Glick"},
	{0x7c741060817f6f47, "'Invariant assertions' is the most elegant programming technique!  -Tom Szymanski"},
	{0x47725a1b8b5d5b23, strings.Repeat("\xff", 5548) + "8"},
	{0x822bf24e44f3cf47, strings.Repeat("\xff", 5549) + "9"},
	{0x0fef60d322a5683c, strings.Repeat("\xff", 5550) + "0"},
	{0xd94e03ac7cb45f2b, strings.Repeat("\xff", 5551) + "1"},
	{0x9bb40268d8686627, strings.Repeat("\xff", 5552) + "2"},
	{0x5106259cbe3ad66f, strings.Repeat("\xff", 5553) + "3"},
	{0x1d78d1488873a6b3, strings.Repeat("\xff", 5554) + "4"},
	{0xb777c3ead542855e, strings.Repeat("\xff", 5555) + "5"},
	{0xb29a7ddef80302ae, strings.Repeat("\x00", 1e5)},
	{0xefc68f5cd613cff4, strings.Repeat("a", 1e5)},
	{0x0c8acc8f6807231d, strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ", 1e4)},
}

// Prove that we implement various standard interfaces.
var (
	_ hash.Hash64        = rollsum.New()
	_ rollinghash.Hash64 = rollsum.New()
	_ io.Writer          = rollsum.New()
)

// Sum64ByWriteAndRoll computes the sum by prepending the input with '\0',
// writing all but the last byte, then rolling the last byte.
func Sum64ByWriteAndRoll(b []byte) uint64 {
	q := []byte("\x00")
	q = append(q, b...)
	roll := rollsum.New()
	if _, err := roll.Write(q[:len(q)-1]); err != nil {
		panic(err)
	}
	roll.Roll(q[len(q)-1])
	return roll.Sum64()
}

func TestGolden(t *testing.T) {
	for _, g := range golden {
		in := g.in

		p := []byte(in)
		classic := hash.Hash64(rollsum.New())
		if _, err := classic.Write(p); err != nil {
			t.Fatal(err)
		}
		if got := classic.Sum64(); got != g.out {
			t.Errorf("classic implementation: for %q, expected 0x%x, got 0x%x", in, g.out, got)
			continue
		}

		if got := Sum64ByWriteAndRoll(p); got != g.out {
			t.Errorf("rolling implementation: for %q, expected 0x%x, got 0x%x", in, g.out, got)
			continue
		}
	}
}

func BenchmarkRolling64B(b *testing.B) {
	b.SetBytes(1)
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
	b.SetBytes(1)
	b.ReportAllocs()
	r := bufio.NewReader(rand.New(rand.NewSource(0)))
	ws := 64
	window := make([]byte, ws)
	n, err := r.Read(window)
	if n != ws || err != nil {
		b.Errorf("could not read %d bytes", ws)
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

func BenchmarkBatchRoll(b *testing.B) {
	const window = 56
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(i*131 + 7)
	}
	dst := make([]uint64, len(data)-window+1)
	h := rollsum.New()

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		h.BatchRoll(dst, data, window)
	}
}

func BenchmarkBatchBoundaries(b *testing.B) {
	const window = 56
	const mask = uint64(0x1fff)
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(i*131 + 7)
	}
	n := len(data) - window + 1
	la := make([]int32, n)
	lb := make([]int32, n)
	h := rollsum.New()

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		h.BatchBoundaries(la, lb, data, window, mask)
	}
}
