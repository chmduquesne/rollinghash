package bozo32_test

import (
	"bufio"
	"hash"
	"io"
	"math"
	"math/bits"
	"math/rand"
	"strings"
	"testing"

	"github.com/chmduquesne/rollinghash/v4"
	rollsum "github.com/chmduquesne/rollinghash/v4/bozo32"
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

// FuzzNewFromInt verifies that for any multiplier a passed to NewFromInt,
// Roll and BatchRoll both agree with a fresh Write+Sum32 at every window position.
func FuzzNewFromInt(f *testing.F) {
	f.Add(uint32(65521), []byte("The quick brown fox jumps over the lazy dog"), 16)
	f.Add(uint32(1), []byte("hello world"), 4)
	f.Add(uint32(0), []byte("abcdef"), 3)
	f.Add(uint32(1<<16-15), []byte("aaaaaa"), 2)

	f.Fuzz(func(t *testing.T, a uint32, data []byte, window int) {
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
			if rolling.Sum32() != classic.Sum32() {
				t.Fatalf("Roll mismatch at pos %d (a=%d): got 0x%x want 0x%x",
					i, a, rolling.Sum32(), classic.Sum32())
			}
		}

		// Verify BatchRoll.
		dst := make([]uint64, len(data)-window+1)
		rollsum.NewFromInt(a).BatchRoll(dst, data, window)
		for i := range dst {
			classic.Reset()
			classic.Write(data[i : i+window])
			if uint32(dst[i]) != classic.Sum32() {
				t.Fatalf("BatchRoll mismatch at pos %d (a=%d): got 0x%x want 0x%x",
					i, a, uint32(dst[i]), classic.Sum32())
			}
		}
	})
}

// FuzzNewFromIntCDC checks that bozo32.NewFromInt(a) produces hash values
// with geometric trailing-zero decay, which is the property that makes a
// rolling hash suitable for CDC (chunk boundaries hit with probability 2^-k
// for a k-bit mask).
//
// Even multipliers are skipped: a^n accumulates factors of 2 for large
// windows, zeroing the low bits and ruining the distribution. a<=1 is also
// skipped: a=1 collapses the hash to a simple sum bounded by window*255,
// leaving the high bits permanently zero.
func FuzzNewFromIntCDC(f *testing.F) {
	f.Add(uint32(65521))       // default: largest prime fitting in 16 bits
	f.Add(uint32(32771))       // a smaller odd prime
	f.Add(uint32(1<<16 + 3))   // large odd

	f.Fuzz(func(t *testing.T, a uint32) {
		if a&1 == 0 || a <= 1 {
			t.Skip()
		}

		const n = 1 << 20
		data := make([]byte, n)
		rng := rand.New(rand.NewSource(42))
		for i := range data {
			data[i] = byte(rng.Uint64())
		}

		const win = 56
		nPos := n - win + 1
		dst := make([]uint64, nPos)
		rollsum.NewFromInt(a).BatchRoll(dst, data, win)

		var tzHist [32]uint64
		for _, v := range dst {
			tzHist[bits.TrailingZeros32(uint32(v))]++
		}

		total := float64(nPos)
		for k := range 20 {
			expected := total * math.Pow(0.5, float64(k+1))
			if expected < 1000 {
				break
			}
			sigma := math.Sqrt(expected)
			if math.Abs(float64(tzHist[k])-expected) > 4*sigma {
				t.Errorf("a=%d: trailing zeros k=%d: observed=%d expected=%.0f (>4σ deviation)",
					a, k, tzHist[k], expected)
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
	f := rand.New(rand.NewSource(0))
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
