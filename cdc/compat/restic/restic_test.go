package restic_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	restic "github.com/chmduquesne/rollinghash/v4/cdc/compat/restic"
)

// resticDefaultPol is restic's historical default polynomial (chunker.Pol),
// still a valid degree-53 irreducible polynomial and handy as a fixed test
// vector.
const resticDefaultPol = restic.Pol(0x3DA3358B4DC173)

func xorshiftData(n int) []byte {
	b := make([]byte, n)
	var x uint64 = 0x9E3779B97F4A7C15
	for i := range b {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		b[i] = byte(x)
	}
	return b
}

func drain(t *testing.T, c *restic.Chunker) []restic.Chunk {
	t.Helper()
	var out []restic.Chunk
	for {
		chunk, err := c.Next(nil)
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		out = append(out, chunk)
	}
}

// TestGoldenBoundaries pins the exact chunk boundaries and Cut values for a
// fixed polynomial, data, and options. The values were produced by the real
// github.com/restic/chunker v0.5.0 and verified byte-identical (see
// cdc/compat/restic/bench). This is the no-external-dependency regression guard.
func TestGoldenBoundaries(t *testing.T) {
	want := []restic.Chunk{
		{0, 735, 0x12943f29d1000, nil},
		{735, 1612, 0x9114782200000, nil},
		{2347, 5061, 0x92a83a1e85000, nil},
		{7408, 811, 0x6514267f3c800, nil},
		{8219, 2497, 0x12d581f366b000, nil},
		{10716, 2306, 0x1b352798a7800, nil},
		{13022, 3755, 0x1c1663b3c62800, nil},
		{16777, 3395, 0x16a0a58f2cd000, nil},
		{20172, 347, 0x1e5fda64cf0000, nil},
		{20519, 1999, 0x1acacd96550800, nil},
		{22518, 3915, 0x1fd0af97c65800, nil},
		{26433, 1480, 0x86c77746ee800, nil},
		{27913, 833, 0x1c4c85fc46b000, nil},
		{28746, 275, 0x3f2d9573af000, nil},
		{29021, 593, 0xf1f81369cd000, nil},
		{29614, 3917, 0x53daa24f01800, nil},
		{33531, 262, 0xf70cd5f182000, nil},
		{33793, 286, 0x1700e627a97000, nil},
		{34079, 360, 0xc761d8cec1000, nil},
		{34439, 3351, 0x66f065ff9e800, nil},
		{37790, 4992, 0x1405eb86475000, nil},
		{42782, 1430, 0x1fb54f604b8800, nil},
		{44212, 3446, 0xc5e94f4835800, nil},
		{47658, 1683, 0x800cf90b6f800, nil},
		{49341, 659, 0x1b83338b883c25, nil},
	}

	data := xorshiftData(50000)
	c := restic.New(bytes.NewReader(data), resticDefaultPol,
		restic.WithBoundaries(256, 8192), restic.WithAverageBits(11))
	got := drain(t, c)

	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d", len(got), len(want))
	}
	var total uint
	for i, g := range got {
		w := want[i]
		if g.Start != w.Start || g.Length != w.Length || g.Cut != w.Cut {
			t.Errorf("chunk %d: {%d, %d, %#x}, want {%d, %d, %#x}",
				i, g.Start, g.Length, g.Cut, w.Start, w.Length, w.Cut)
		}
		if !bytes.Equal(g.Data, data[g.Start:g.Start+g.Length]) {
			t.Errorf("chunk %d: Data does not match the source slice", i)
		}
		total += g.Length
	}
	if total != uint(len(data)) {
		t.Errorf("chunks cover %d bytes, want %d", total, len(data))
	}
}

func TestNextReusesBuffer(t *testing.T) {
	data := xorshiftData(40000)
	c := restic.New(bytes.NewReader(data), resticDefaultPol,
		restic.WithBoundaries(256, 8192), restic.WithAverageBits(11))

	buf := make([]byte, 0, 8192)
	var reassembled []byte
	for {
		chunk, err := c.Next(buf)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(chunk.Data) > 0 && &chunk.Data[0] != &buf[:1][0] {
			t.Fatalf("Next did not append into the caller's buffer")
		}
		reassembled = append(reassembled, chunk.Data...)
		buf = chunk.Data
	}
	if !bytes.Equal(reassembled, data) {
		t.Fatalf("reassembled %d bytes, want %d", len(reassembled), len(data))
	}
}

func TestEOFIsSticky(t *testing.T) {
	c := restic.New(bytes.NewReader(xorshiftData(5000)), resticDefaultPol,
		restic.WithBoundaries(256, 8192), restic.WithAverageBits(11))
	for {
		_, err := c.Next(nil)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := range 3 {
		if _, err := c.Next(nil); !errors.Is(err, io.EOF) {
			t.Fatalf("call %d past EOF: err = %v, want io.EOF", i, err)
		}
	}
}

func TestSubWindowAndEmpty(t *testing.T) {
	// Whole stream shorter than the 64-byte window: one final chunk of all the
	// bytes, Cut == 0 (documented difference from restic, which returns a
	// meaningless non-split digest there).
	tiny := xorshiftData(10)
	c := restic.New(bytes.NewReader(tiny), resticDefaultPol)
	chunk, err := c.Next(nil)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !bytes.Equal(chunk.Data, tiny) || chunk.Start != 0 || chunk.Length != 10 || chunk.Cut != 0 {
		t.Fatalf("tiny: {%d, %d, %#x} %q", chunk.Start, chunk.Length, chunk.Cut, chunk.Data)
	}
	if _, err := c.Next(nil); !errors.Is(err, io.EOF) {
		t.Fatalf("tiny: second Next err = %v, want io.EOF", err)
	}

	// Empty stream: no chunks.
	c = restic.New(bytes.NewReader(nil), resticDefaultPol)
	if _, err := c.Next(nil); !errors.Is(err, io.EOF) {
		t.Fatalf("empty: err = %v, want io.EOF", err)
	}
}

func TestReset(t *testing.T) {
	data := xorshiftData(60000)
	c := restic.New(bytes.NewReader(data), resticDefaultPol,
		restic.WithBoundaries(256, 8192), restic.WithAverageBits(11))
	first := drain(t, c)

	c.Reset(bytes.NewReader(data), resticDefaultPol,
		restic.WithBoundaries(256, 8192), restic.WithAverageBits(11))
	second := drain(t, c)

	if len(first) != len(second) {
		t.Fatalf("after Reset: %d chunks, first pass had %d", len(second), len(first))
	}
	for i := range first {
		if first[i].Start != second[i].Start || first[i].Length != second[i].Length || first[i].Cut != second[i].Cut {
			t.Fatalf("chunk %d differs after Reset", i)
		}
	}
}

func TestReaderError(t *testing.T) {
	boom := errors.New("boom")
	c := restic.New(iotest.ErrReader(boom), resticDefaultPol)
	if _, err := c.Next(nil); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestNewPanicsOnBadPolynomial(t *testing.T) {
	for _, pol := range []restic.Pol{0, 1 << 54} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("New(pol=%#x) did not panic", uint64(pol))
				}
			}()
			restic.New(bytes.NewReader(nil), pol)
		}()
	}
}

func TestPolJSONRoundTrip(t *testing.T) {
	// restic stores the repository polynomial as a quoted lowercase-hex string
	// with no "0x" prefix.
	b, err := json.Marshal(resticDefaultPol)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"3da3358b4dc173"` {
		t.Fatalf("Marshal = %s, want \"3da3358b4dc173\"", b)
	}
	var p restic.Pol
	if err := json.Unmarshal([]byte(`"3da3358b4dc173"`), &p); err != nil {
		t.Fatal(err)
	}
	if p != resticDefaultPol {
		t.Fatalf("Unmarshal = %#x, want %#x", uint64(p), uint64(resticDefaultPol))
	}
}

func TestRandomPolynomial(t *testing.T) {
	p, err := restic.RandomPolynomial()
	if err != nil {
		t.Fatal(err)
	}
	if p.Deg() != 53 || !p.Irreducible() {
		t.Fatalf("RandomPolynomial returned %#x (deg %d, irreducible %v)", uint64(p), p.Deg(), p.Irreducible())
	}
	// It must actually drive a chunker.
	c := restic.New(bytes.NewReader(xorshiftData(4096)), p, restic.WithBoundaries(64, 1024), restic.WithAverageBits(8))
	if drain(t, c) == nil {
		t.Fatal("no chunks from a RandomPolynomial chunker")
	}
}
