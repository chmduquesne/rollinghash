package buildbarn_test

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"testing"

	cdc "github.com/chmduquesne/rollinghash/v4/cdc/compat/buildbarn"
)

func randData(n int) []byte {
	data := make([]byte, n)
	var x uint64 = 0x9e3779b97f4a7c15
	for i := range data {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		data[i] = byte(x)
	}
	return data
}

func drain(t *testing.T, r cdc.ChunkReader) [][]byte {
	t.Helper()
	var out [][]byte
	for {
		b, err := r.ReadNextChunk()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("ReadNextChunk: %v", err)
		}
		out = append(out, append([]byte(nil), b...))
	}
}

func lens(chunks [][]byte) []int {
	out := make([]int, len(chunks))
	for i, c := range chunks {
		out[i] = len(c)
	}
	return out
}

func peek(cd cdc.ContentDefinedChunker, data []byte) cdc.ChunkReader {
	return cd.NewChunkReader(bufio.NewReaderSize(bytes.NewReader(data), 64<<10))
}

var gt = &cdc.FastContentDefinedChunkerGearTable

func TestGoldenBoundaries(t *testing.T) {
	data := randData(96 * 1024)
	cases := []struct {
		name string
		cd   cdc.ContentDefinedChunker
		want []int
	}{
		{"fast", cdc.NewFastContentDefinedChunker(gt, 4096),
			[]int{5978, 4111, 5338, 2748, 4717, 4233, 4585, 4400, 6655, 4730, 4139, 4331, 5526, 6726, 5418, 4782, 4461, 6038, 4263, 4106, 1019}},
		{"max", cdc.NewMaxContentDefinedChunker(gt, 2048, 8192),
			[]int{2532, 7455, 7975, 8175, 2057, 8053, 4382, 2266, 7615, 5408, 5490, 7176, 3042, 7596, 3982, 2286, 6569, 2998, 3247}},
		{"repmax", cdc.NewRepMaxContentDefinedChunker(gt, 4096, 4096),
			[]int{7903, 5898, 4161, 8175, 6783, 7709, 6740, 5682, 5840, 5016, 7719, 7596, 6268, 6569, 6245}},
		{"repmaxsfx", cdc.NewRepMaxSfxContentDefinedChunker(&cdc.NoSubstitutionBox, 4096, 4096),
			[]int{6353, 4554, 6821, 5680, 6843, 6900, 6849, 7693, 7510, 4755, 4682, 6589, 6572, 5846, 5888, 4769}},
		{"ae", cdc.NewAsymmetricExtremumContentDefinedChunker(4096, 16384),
			[]int{4112, 4176, 4107, 4489, 4586, 4166, 4192, 4181, 4577, 4166, 4219, 4322, 4382, 4124, 4252, 4198, 4335, 4516, 4266, 4202, 4133, 4141, 4111, 351}},
	}
	for _, tc := range cases {
		got := drain(t, peek(tc.cd, data))
		if joined := bytes.Join(got, nil); !bytes.Equal(joined, data) {
			t.Fatalf("%s: reassembled %d bytes, want %d", tc.name, len(joined), len(data))
		}
		if !equalInts(lens(got), tc.want) {
			t.Errorf("%s: lengths %v, want %v", tc.name, lens(got), tc.want)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSimpleEqualsOptimized(t *testing.T) {
	data := randData(200 * 1024)
	pairs := []struct {
		name string
		a, b cdc.ContentDefinedChunker
	}{
		{"max", cdc.NewMaxContentDefinedChunker(gt, 2048, 8192), cdc.NewSimpleMaxContentDefinedChunker(gt, 2048, 8192)},
		{"repmax", cdc.NewRepMaxContentDefinedChunker(gt, 4096, 8192), cdc.NewSimpleRepMaxContentDefinedChunker(gt, 4096, 8192)},
		{"repmaxsfx", cdc.NewRepMaxSfxContentDefinedChunker(&cdc.NoSubstitutionBox, 4096, 8192), cdc.NewSimpleRepMaxSfxContentDefinedChunker(4096, 8192)},
		{"ae", cdc.NewAsymmetricExtremumContentDefinedChunker(4096, 16384), cdc.NewSimpleAsymmetricExtremumContentDefinedChunker(4096, 16384)},
	}
	for _, p := range pairs {
		if !equalInts(lens(drain(t, peek(p.a, data))), lens(drain(t, peek(p.b, data)))) {
			t.Errorf("%s: Simple and non-Simple disagree", p.name)
		}
	}
}

func TestMaximumPeekSize(t *testing.T) {
	for _, tc := range []struct {
		got, want int
	}{
		{cdc.NewFastContentDefinedChunker(gt, 4096).GetMaximumPeekSizeBytes(), 4 * 4096},
		{cdc.NewMaxContentDefinedChunker(gt, 2048, 8192).GetMaximumPeekSizeBytes(), 2048 + 8192},
		{cdc.NewRepMaxContentDefinedChunker(gt, 4096, 4096).GetMaximumPeekSizeBytes(), 2*4096 + 4096},
		{cdc.NewRepMaxSfxContentDefinedChunker(nil, 4096, 4096).GetMaximumPeekSizeBytes(), 2*4096 + 4096},
		{cdc.NewAsymmetricExtremumContentDefinedChunker(4096, 16384).GetMaximumPeekSizeBytes(), 16384},
	} {
		if tc.got != tc.want {
			t.Errorf("GetMaximumPeekSizeBytes = %d, want %d", tc.got, tc.want)
		}
	}
}

func TestSyncNotSupported(t *testing.T) {
	cd := cdc.NewMaxContentDefinedChunker(gt, 2048, 8192)
	if cd.SupportsDiscardUpToGuaranteedChunk() {
		t.Fatal("SupportsDiscardUpToGuaranteedChunk should be false")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("DiscardUpToGuaranteedChunk should panic")
		}
	}()
	_ = cd.DiscardUpToGuaranteedChunk(bufio.NewReader(bytes.NewReader(nil)))
}

func TestEmptyAndEOFSticky(t *testing.T) {
	r := peek(cdc.NewMaxContentDefinedChunker(gt, 2048, 8192), nil)
	for i := range 3 {
		if b, err := r.ReadNextChunk(); b != nil || err != io.EOF {
			t.Fatalf("empty read %d: %v bytes, err %v; want nil, io.EOF", i, len(b), err)
		}
	}
}

type errPeeker struct{ err error }

func (e errPeeker) Peek(int) ([]byte, error) { return nil, e.err }
func (e errPeeker) Discard(int) (int, error) { return 0, e.err }

func TestPeekerErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	// errPeeker is not an io.Reader, so the Peek+Discard shim is exercised.
	r := cdc.NewMaxContentDefinedChunker(gt, 2048, 8192).NewChunkReader(errPeeker{boom})
	if _, err := r.ReadNextChunk(); !errors.Is(err, boom) {
		t.Fatalf("ReadNextChunk err = %v, want boom", err)
	}
}

func TestFastPanicsOnBadNormalSize(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for normalSize 3000")
		}
	}()
	cdc.NewFastContentDefinedChunker(gt, 3000)
}

func TestSeededTablesDeterministic(t *testing.T) {
	a := cdc.NewSeededGearTable([]byte("key"))
	b := cdc.NewSeededGearTable([]byte("key"))
	data := randData(64 * 1024)
	x := lens(drain(t, peek(cdc.NewMaxContentDefinedChunker(a, 2048, 8192), data)))
	y := lens(drain(t, peek(cdc.NewMaxContentDefinedChunker(b, 2048, 8192), data)))
	if !equalInts(x, y) {
		t.Fatal("NewSeededGearTable is not deterministic")
	}
	if equalInts(x, lens(drain(t, peek(cdc.NewMaxContentDefinedChunker(gt, 2048, 8192), data)))) {
		t.Fatal("seeded table produced the same boundaries as the default table")
	}

	sb := cdc.NewSeededSubstitutionBox([]byte("key"))
	if *sb == cdc.NoSubstitutionBox {
		t.Fatal("NewSeededSubstitutionBox returned the identity box")
	}
}

func TestRepMaxSfxSubstitutionBox(t *testing.T) {
	data := randData(120 * 1024)
	sb := cdc.NewSeededSubstitutionBox([]byte("sbox"))

	// Boxed run over data == identity run over the remapped data.
	remapped := make([]byte, len(data))
	for i, b := range data {
		remapped[i] = sb[b]
	}
	boxed := lens(drain(t, peek(cdc.NewRepMaxSfxContentDefinedChunker(sb, 4096, 8192), data)))
	plain := lens(drain(t, peek(cdc.NewSimpleRepMaxSfxContentDefinedChunker(4096, 8192), remapped)))
	if !equalInts(boxed, plain) {
		t.Fatalf("substitution box: boxed lengths %v, want %v", boxed, plain)
	}
}
