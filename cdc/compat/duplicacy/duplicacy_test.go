package duplicacy_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/bits"
	"testing"
	"testing/iotest"

	"github.com/chmduquesne/rollinghash/v4/buzhash64"
	duplicacy "github.com/chmduquesne/rollinghash/v4/cdc/compat/duplicacy"
)

// --- independent reference implementation of Duplicacy's chunking algorithm ---
//
// This is written from the algorithm (buzhash cyclic polynomial, window =
// minimumChunkSize, single mask averageChunkSize-1, clamp to [min,max], SHA-256
// table chain), not adapted from Duplicacy's source. It is the oracle the
// compat package is checked against, since Duplicacy publishes no Go module
// that still builds in isolation.

func refTable(seed []byte) (t [256]uint64) {
	d := sha256.Sum256(seed)
	for i := range 64 {
		for j := range 4 {
			t[4*i+j] = binary.LittleEndian.Uint64(d[8*j : 8*j+8])
		}
		d = sha256.Sum256(d[:])
	}
	return t
}

type span struct{ start, length int }

func refChunks(seed []byte, avg, minSz, maxSz int, data []byte) []span {
	tbl := refTable(seed)
	mask := uint64(avg - 1)
	rot := minSz % 64

	rotl1 := func(v uint64) uint64 { return v<<1 | v>>63 }
	winHash := func(w []byte) uint64 {
		var s uint64
		for _, b := range w {
			s = rotl1(s) ^ tbl[b]
		}
		return s
	}

	var out []span
	start := 0
	for start < len(data) {
		if len(data)-start < minSz {
			out = append(out, span{start, len(data) - start})
			return out
		}
		h := winHash(data[start : start+minSz])
		cut := -1
		if h&mask == 0 {
			cut = start + minSz
		} else {
			limit := min(start+maxSz, len(data))
			for pos := start + minSz; pos < limit; pos++ {
				h = rotl1(h) ^ bits.RotateLeft64(tbl[data[pos-minSz]], rot) ^ tbl[data[pos]]
				if h&mask == 0 || pos+1 == start+maxSz {
					cut = pos + 1
					break
				}
			}
		}
		if cut < 0 { // ran out of data before any boundary: trailing chunk
			out = append(out, span{start, len(data) - start})
			return out
		}
		out = append(out, span{start, cut - start})
		start = cut
	}
	return out
}

// --- helpers ---

func xorshift(n int, seed uint64) []byte {
	b := make([]byte, n)
	x := seed
	for i := range b {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		b[i] = byte(x)
	}
	return b
}

func lowEntropy(n int, seed uint64) []byte {
	b := xorshift(n, seed)
	for i := n / 3; i < 2*n/3 && i < len(b); i++ {
		b[i] = 0x5c
	}
	return b
}

func drainSpans(t *testing.T, seed []byte, avg, minSz, maxSz int, data []byte) []span {
	t.Helper()
	m := duplicacy.CreateChunkMaker(
		duplicacy.WithChunkSeed(seed),
		duplicacy.WithAverageChunkSize(avg),
		duplicacy.WithMinimumChunkSize(minSz),
		duplicacy.WithMaximumChunkSize(maxSz),
	)
	var got []span
	collect := func(c duplicacy.Chunk) {
		if c.Length != len(c.Data) {
			t.Fatalf("Chunk.Length %d != len(Data) %d", c.Length, len(c.Data))
		}
		if !bytes.Equal(c.Data, data[c.Start:c.Start+c.Length]) {
			t.Fatalf("chunk at %d does not match the source slice", c.Start)
		}
		got = append(got, span{c.Start, c.Length})
	}
	if err := m.AddData(bytes.NewReader(data), collect); err != nil {
		t.Fatalf("AddData: %v", err)
	}
	if err := m.AddData(nil, collect); err != nil {
		t.Fatalf("AddData(nil): %v", err)
	}
	return got
}

// --- tests ---

// TestTableGolden anchors the reference table's derivation with a hand
// computation of its first entry: SHA-256 of the seed, first 8 bytes read
// little-endian. The compat package's own deriveTable is then covered
// transitively — if it disagreed with the reference, TestParityReference would
// fail.
func TestTableGolden(t *testing.T) {
	d := sha256.Sum256([]byte("duplicacy"))
	want := binary.LittleEndian.Uint64(d[:8])
	if got := refTable([]byte("duplicacy"))[0]; got != want {
		t.Fatalf("refTable[0] = %#x, want %#x", got, want)
	}
}

// TestParityReference is the core proof: for a grid of seeds, size triples and
// data shapes the compat ChunkMaker must produce exactly the spans the
// independent reference does.
func TestParityReference(t *testing.T) {
	seeds := [][]byte{[]byte("duplicacy"), []byte("another-repo-seed"), {0x00}, {}}
	shapes := map[string]func(int, uint64) []byte{"rand": xorshift, "struct": lowEntropy}
	sizes := []int{0, 1, 63, 64, 65, 200, 999, 4096, 50_000, 262_144, 1 << 20}
	configs := []struct{ avg, min, max int }{
		{256, 64, 1024},
		{512, 128, 2048},
		{1024, 256, 4096},
		{4096, 1024, 16384},
		{8192, 2048, 65536},
		{1 << 16, 1 << 14, 1 << 18},
	}

	for si, seed := range seeds {
		for shapeName, gen := range shapes {
			for _, n := range sizes {
				data := gen(n, 0x9E3779B97F4A7C15+uint64(si))
				for _, cfg := range configs {
					want := refChunks(seed, cfg.avg, cfg.min, cfg.max, data)
					got := drainSpans(t, seed, cfg.avg, cfg.min, cfg.max, data)
					if len(got) != len(want) {
						t.Fatalf("seed=%q %s n=%d %v: %d chunks, want %d\n got=%v\nwant=%v",
							seed, shapeName, n, cfg, len(got), len(want), got, want)
					}
					var total int
					for i := range want {
						if got[i] != want[i] {
							t.Fatalf("seed=%q %s n=%d %v chunk %d: %v != %v",
								seed, shapeName, n, cfg, i, got[i], want[i])
						}
						total += got[i].length
					}
					if total != n {
						t.Fatalf("seed=%q %s n=%d %v: chunks cover %d bytes", seed, shapeName, n, cfg, total)
					}
				}
			}
		}
	}
}

// TestGoldenBoundaries pins exact spans for a fixed seed, data and size triple,
// so a regression shows up even if the reference above were changed in lockstep.
func TestGoldenBoundaries(t *testing.T) {
	want := []span{
		{0, 308}, {308, 336}, {644, 150}, {794, 319}, {1113, 229},
		{1342, 178}, {1520, 177}, {1697, 466}, {2163, 202}, {2365, 527},
		{2892, 552}, {3444, 125}, {3569, 1024}, {4593, 147}, {4740, 267},
		{5007, 277}, {5284, 436}, {5720, 1024}, {6744, 349}, {7093, 109},
	}
	data := xorshift(7202, 0x1234567890ABCDEF)
	got := drainSpans(t, []byte("duplicacy"), 256, 64, 1024, data)
	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk %d: %v, want %v", i, got[i], want[i])
		}
	}
}

func TestSplitAcrossReaders(t *testing.T) {
	data := xorshift(40_000, 0xABCDEF)
	whole := drainSpans(t, []byte("duplicacy"), 512, 128, 4096, data)

	m := duplicacy.CreateChunkMaker(
		duplicacy.WithChunkSeed([]byte("duplicacy")),
		duplicacy.WithAverageChunkSize(512),
		duplicacy.WithMinimumChunkSize(128),
		duplicacy.WithMaximumChunkSize(4096),
	)
	var got []span
	collect := func(c duplicacy.Chunk) { got = append(got, span{c.Start, c.Length}) }
	for _, part := range [][]byte{data[:1], data[1:9999], data[9999:10000], data[10000:33333], data[33333:]} {
		if err := m.AddData(bytes.NewReader(part), collect); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.AddData(nil, collect); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(whole) {
		t.Fatalf("split feed: %d chunks, whole feed: %d", len(got), len(whole))
	}
	for i := range whole {
		if got[i] != whole[i] {
			t.Fatalf("chunk %d: split %v != whole %v", i, got[i], whole[i])
		}
	}
}

// TestChunkHash checks Chunk.Hash: the buzhash of the window (= minimum chunk
// size) ending at each cut, recomputed independently from the seeded table.
func TestChunkHash(t *testing.T) {
	seed := []byte("duplicacy")
	const avg, minSz, maxSz = 1024, 256, 4096
	tbl := refTable(seed)
	windowHash := func(win []byte) uint64 {
		h := buzhash64.NewFromUint64Array(tbl)
		h.Write(win)
		return h.Sum64()
	}

	for _, n := range []int{300, 5000, 200_000} {
		data := xorshift(n, 0xC0FFEE+uint64(n))
		m := duplicacy.CreateChunkMaker(
			duplicacy.WithChunkSeed(seed),
			duplicacy.WithAverageChunkSize(avg),
			duplicacy.WithMinimumChunkSize(minSz),
			duplicacy.WithMaximumChunkSize(maxSz),
		)
		var chunks []duplicacy.Chunk
		collect := func(c duplicacy.Chunk) {
			c.Data = append([]byte(nil), c.Data...)
			chunks = append(chunks, c)
		}
		if err := m.AddData(bytes.NewReader(data), collect); err != nil {
			t.Fatal(err)
		}
		if err := m.AddData(nil, collect); err != nil {
			t.Fatal(err)
		}

		for i, c := range chunks {
			e := c.Start + c.Length - 1 // last byte of the chunk = the cut
			if e-minSz+1 < 0 {
				if c.Hash != 0 {
					t.Fatalf("n=%d chunk %d shorter than window: Hash %#x, want 0", n, i, c.Hash)
				}
				continue
			}
			want := windowHash(data[e-minSz+1 : e+1])
			last := i == len(chunks)-1
			if c.Hash != want && !(last && c.Hash == 0) {
				t.Fatalf("n=%d chunk %d (cut %d, len %d): Hash %#x, want %#x", n, i, e, c.Length, c.Hash, want)
			}
			// A content-defined boundary's hash must satisfy the mask.
			if c.Length < maxSz && !last && c.Hash&uint64(avg-1) != 0 {
				t.Fatalf("n=%d chunk %d: content-defined but Hash %#x & mask != 0", n, i, c.Hash)
			}
		}
	}
}

func TestReset(t *testing.T) {
	data := xorshift(60_000, 0x55)
	m := duplicacy.CreateChunkMaker(duplicacy.WithAverageChunkSize(4096),
		duplicacy.WithMinimumChunkSize(1024), duplicacy.WithMaximumChunkSize(16384))

	run := func() []span {
		var s []span
		cb := func(c duplicacy.Chunk) { s = append(s, span{c.Start, c.Length}) }
		if err := m.AddData(bytes.NewReader(data), cb); err != nil {
			t.Fatal(err)
		}
		if err := m.AddData(nil, cb); err != nil {
			t.Fatal(err)
		}
		return s
	}
	first := run()
	m.Reset()
	second := run()
	if len(first) != len(second) {
		t.Fatalf("after Reset: %d chunks, first pass %d", len(second), len(first))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("chunk %d differs after Reset", i)
		}
	}
}

func TestReaderError(t *testing.T) {
	boom := errors.New("boom")
	m := duplicacy.CreateChunkMaker()
	if err := m.AddData(iotest.ErrReader(boom), func(duplicacy.Chunk) {}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestClosedIsSticky(t *testing.T) {
	m := duplicacy.CreateChunkMaker()
	if err := m.AddData(nil, func(duplicacy.Chunk) {}); err != nil {
		t.Fatal(err)
	}
	if err := m.AddData(bytes.NewReader([]byte("x")), func(duplicacy.Chunk) {}); err == nil {
		t.Fatal("expected an error feeding data after the nil-reader flush")
	}
	m.Reset()
	if err := m.AddData(bytes.NewReader([]byte("x")), func(duplicacy.Chunk) {}); err != nil {
		t.Fatalf("AddData after Reset: %v", err)
	}
}

func TestEmptyStream(t *testing.T) {
	got := drainSpans(t, []byte("duplicacy"), 256, 64, 1024, nil)
	if got != nil {
		t.Fatalf("empty stream produced %v", got)
	}
}

func TestBadParams(t *testing.T) {
	cases := []func(){
		func() { duplicacy.CreateChunkMaker(duplicacy.WithAverageChunkSize(1000)) },            // not power of two
		func() { duplicacy.CreateChunkMaker(duplicacy.WithMinimumChunkSize(32)) },              // window too small
		func() { duplicacy.CreateChunkMaker(duplicacy.WithMinimumChunkSize(8 * 1024 * 1024)) }, // min > avg
	}
	for i, fn := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("case %d did not panic", i)
				}
			}()
			fn()
		}()
	}
}

func BenchmarkChunkMaker(b *testing.B) {
	data := xorshift(16<<20, 0x2545F4914F6CDD1D)
	m := duplicacy.CreateChunkMaker()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		m.Reset()
		sink := func(duplicacy.Chunk) {}
		if err := m.AddData(bytes.NewReader(data), sink); err != nil {
			b.Fatal(err)
		}
		if err := m.AddData(nil, sink); err != nil {
			b.Fatal(err)
		}
	}
}
