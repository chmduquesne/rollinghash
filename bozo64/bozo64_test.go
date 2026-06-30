package bozo64_test

import (
	"bufio"
	"hash"
	"io"
	"math/rand"
	"strings"
	"testing"

	"github.com/chmduquesne/rollinghash/v4"
	rollsum "github.com/chmduquesne/rollinghash/v4/bozo64"
)

var golden = []struct {
	out uint64
	in  string
}{
	//{0x0, ""}, // panics
	{0x61, "a"},
	{0x60fffffe7d, "ab"},
	{0xfffffc98000007f2, "abc"},
	{0x18f9ffffd8aa, "abcd"},
	{0xffff5bc80000c513, "abcde"},
	{0x3fa2afffc2707, "abcdef"},
	{0xffe8443000133d44, "abcdefg"},
	{0x89e853ff9fce14, "abcdefgh"},
	{0xfcee447001e0fa05, "abcdefghi"},
	{0x1139a3d4f69b1e51, "abcdefghij"},
	{0xf6e9ef38d1dbf2d, "Discard medicine more than two years old."},
	{0x85a384667e4e0250, "He who has a shady past knows that nice guys finish last."},
	{0x3ee74649df2a2d5a, "I wouldn't marry him with a ten foot pole."},
	{0xd4c640d5306e95a6, "Free! Free!/A trip/to Mars/for 900/empty jars/Burma Shave"},
	{0xd2dc16889dfe0571, "The days of the digital watch are numbered.  -Tom Stoppard"},
	{0xf351aa37e88b62b1, "Nepal premier won't resign."},
	{0x5e3517f76202390c, "For every action there is an equal and opposite government program."},
	{0x2ed8f40e85653e5, "His money is twice tainted: 'taint yours and 'taint mine."},
	{0xa5b719a1bbb02f86, "There is no reason for any individual to have a computer in their home. -Ken Olsen, 1977"},
	{0x597a75a4dfb060a7, "It's a tiny change to the code and not completely disgusting. - Bob Manchek"},
	{0x7d6419c137d52334, "size:  a.out:  bad magic"},
	{0xc8759fafb761dd76, "The major problem is with sendmail.  -Mark Horton"},
	{0x18061ab5f530afcf, "Give me a rock, paper and scissors and I will move the world.  CCFestoon"},
	{0xcb7a1e039f5bbff8, "If the enemy is within range, then so are you."},
	{0xc33c4a54fd6a0b22, "It's well we cannot hear the screams/That we create in others' dreams."},
	{0x5a24005851a28754, "You remind me of a TV show, but that's all right: I watch it anyway."},
	{0xe0f170035b2a7d62, "C is as portable as Stonehedge!!"},
	{0x47813a73956ec299, "Even if I could be Shakespeare, I think I should still choose to be Faraday. - A. Huxley"},
	{0x23d8914d2591419c, "The fugacity of a constituent in a mixture of gases at a given temperature is proportional to its mole fraction.  Lewis-Randall Rule"},
	{0xa4f7a561c33bd3d5, "How can you write a big system without C++?  -Paul Glick"},
	{0x90e05a60b60af489, "'Invariant assertions' is the most elegant programming technique!  -Tom Szymanski"},
	{0x4d7356d9e893d660, strings.Repeat("\xff", 5548) + "8"},
	{0x655324e5751ccc76, strings.Repeat("\xff", 5549) + "9"},
	{0x7a7d14c0b66ffe04, strings.Repeat("\xff", 5550) + "0"},
	{0x51fe970f6fd00612, strings.Repeat("\xff", 5551) + "1"},
	{0xd5d71392d0efddd2, strings.Repeat("\xff", 5552) + "2"},
	{0xa3bc7cc0eb50a718, strings.Repeat("\xff", 5553) + "3"},
	{0xb8a2381f676cb8c0, strings.Repeat("\xff", 5554) + "4"},
	{0xcc41a0edfae0607e, strings.Repeat("\xff", 5555) + "5"},
	{0x0, strings.Repeat("\x00", 1e5)},
	{0x10b267606733f9c0, strings.Repeat("a", 1e5)},
	{0x455ea239db023ad0, strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ", 1e4)},
}

// Prove that we implement various standard interfaces
var (
	_ hash.Hash64        = rollsum.New()
	_ rollinghash.Hash64 = rollsum.New()
	_ io.Writer          = rollsum.New()
)

// Sum64ByWriteAndRoll computes the sum by prepending the input slice with
// a '\0', writing the first bytes of this slice into the sum, then
// sliding on the last byte and returning the result of Sum64
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

		// We test the classic implementation
		p := []byte(g.in)
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

// FuzzNewFromInt verifies that for any multiplier a passed to NewFromInt,
// Roll and BatchRoll both agree with a fresh Write+Sum64 at every window position.
func FuzzNewFromInt(f *testing.F) {
	f.Add(uint64(4294967291), []byte("The quick brown fox jumps over the lazy dog"), 16)
	f.Add(uint64(1), []byte("hello world"), 4)
	f.Add(uint64(0), []byte("abcdef"), 3)
	f.Add(uint64(1<<32-5), []byte("aaaaaa"), 2)

	f.Fuzz(func(t *testing.T, a uint64, data []byte, window int) {
		if window < 1 || window > len(data) {
			return
		}

		classic := rollsum.NewFromInt(a)

		// Verify Roll.
		rolling := rollsum.NewFromInt(a)
		rolling.Write(data[:window])
		for i := window; i < len(data); i++ {
			rolling.Roll(data[i])
			classic.Reset()
			classic.Write(data[i-window+1 : i+1])
			if rolling.Sum64() != classic.Sum64() {
				t.Fatalf("Roll mismatch at pos %d (a=%d): got 0x%x want 0x%x",
					i, a, rolling.Sum64(), classic.Sum64())
			}
		}

		// Verify BatchRoll.
		dst := make([]uint64, len(data)-window+1)
		rollsum.NewFromInt(a).BatchRoll(dst, data, window)
		for i := range dst {
			classic.Reset()
			classic.Write(data[i : i+window])
			if dst[i] != classic.Sum64() {
				t.Fatalf("BatchRoll mismatch at pos %d (a=%d): got 0x%x want 0x%x",
					i, a, dst[i], classic.Sum64())
			}
		}
	})
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
