package rollinghash_test

import (
	"bytes"
	"hash"
	"testing"

	"github.com/chmduquesne/rollinghash/v4"
	_adler32 "github.com/chmduquesne/rollinghash/v4/adler32"
	"github.com/chmduquesne/rollinghash/v4/bozo32"
	"github.com/chmduquesne/rollinghash/v4/bozo64"
	"github.com/chmduquesne/rollinghash/v4/buzhash32"
	"github.com/chmduquesne/rollinghash/v4/buzhash64"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
	"github.com/chmduquesne/rollinghash/v4/rabinkarp64"
)

// batchRoller mirrors the unexported rollinghash.batchRoller interface so the
// external test package can type-assert and call it directly.
type batchRoller interface {
	BatchRoll(dst []uint64, data []byte, window int)
}

// boundaryRoller mirrors the unexported rollinghash.boundaryRoller interface.
type boundaryRoller interface {
	BatchBoundaries(a, b []int32, data []byte, window int, mask uint64) (na, nb int)
}

var allHashes = []struct {
	name    string
	classic hash.Hash
	rolling rollinghash.Hash
	new     func() rollinghash.Hash
}{
	{"adler32", _adler32.New(), _adler32.New(), func() rollinghash.Hash { return _adler32.New() }},
	{"buzhash32", buzhash32.New(), buzhash32.New(), func() rollinghash.Hash { return buzhash32.New() }},
	{"buzhash64", buzhash64.New(), buzhash64.New(), func() rollinghash.Hash { return buzhash64.New() }},
	{"bozo32", bozo32.New(), bozo32.New(), func() rollinghash.Hash { return bozo32.New() }},
	{"bozo64", bozo64.New(), bozo64.New(), func() rollinghash.Hash { return bozo64.New() }},
	{"gearhash64", gearhash64.New(), gearhash64.New(), func() rollinghash.Hash { return gearhash64.New() }},
	{"rabinkarp64", rabinkarp64.New(), rabinkarp64.New(), func() rollinghash.Hash { return rabinkarp64.New() }},
}

// Gets the hash sum as a uint64
func sum64(h hash.Hash) (res uint64) {
	buf := make([]byte, 0, 8)
	s := h.Sum(buf)
	for _, b := range s {
		res <<= 8
		res |= uint64(b)
	}
	return
}

// Roll a window of 16 bytes with a classic hash and a rolling hash and
// compare the results
func foxDog(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	s := []byte("The quick brown fox jumps over the lazy dog")

	// Window len
	n := 16

	// Load the window into the rolling hash
	rolling.Write(s[:n])

	// Roll it and compare the result with full re-calculus every time
	for i := n; i < len(s); i++ {

		// Reset and write the window in classic
		classic.Reset()
		classic.Write(s[i-n+1 : i+1])

		// Roll the incoming byte in rolling
		rolling.Roll(s[i])

		// Compare the hashes
		sumc := sum64(classic)
		sumr := sum64(rolling)
		if sumc != sumr {
			t.Errorf("[%s] %v: expected %x, got %x",
				hashname, s[i-n+1:i+1], sumc, sumr)
		}
	}
}

func rollEmptyWindow(t *testing.T, hashname string, _ hash.Hash, rolling rollinghash.Hash) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("[%s] Rolling an empty window should cause a panic", hashname)
		}
	}()
	// This should panic
	rolling.Roll(byte('x'))
}

func writeTwice(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	rolling.Write([]byte("hello "))
	rolling.Write([]byte("world"))

	classic.Write([]byte("hello world"))

	if sum64(rolling) != sum64(classic) {
		t.Errorf("[%s] Expected same results on rolling and classic", hashname)
	}
}

func writeTwiceThenRoll(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	// Build the window across two Write calls, then verify Roll is correct.
	// The window after both writes is "hello world" (11 bytes); rolling '!'
	// should drop 'h' and produce the hash of "ello world!".
	rolling.Write([]byte("hello "))
	rolling.Write([]byte("world"))
	rolling.Roll(byte('!'))

	classic.Reset()
	classic.Write([]byte("ello world!"))

	if sum64(rolling) != sum64(classic) {
		t.Errorf("[%s] Expected same results after two writes then roll", hashname)
	}
}

func writeRollWrite(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	rolling.Write([]byte(" hello"))
	rolling.Roll(byte(' '))
	rolling.Write([]byte("world"))

	classic.Write([]byte("hello world"))

	if sum64(rolling) != sum64(classic) {
		t.Errorf("[%s] Expected same results on rolling and classic", hashname)
	}
}

func writeThenWriteNothing(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	rolling.Write([]byte("hello"))
	rolling.Write([]byte(""))

	classic.Write([]byte("hello"))

	if sum64(rolling) != sum64(classic) {
		t.Errorf("[%s] Expected same results on rolling and classic", hashname)
	}
}

func writeNothing(t *testing.T, hashname string, classic hash.Hash, rolling rollinghash.Hash) {
	rolling.Write([]byte(""))

	if sum64(rolling) != sum64(classic) {
		t.Errorf("[%s] Expected same results on rolling and classic", hashname)
	}
}

func read(t *testing.T, hashname string, rolling rollinghash.Hash) {
	rolling.Write([]byte("hello "))

	var buf bytes.Buffer
	readWindow := func() []byte {
		buf.Reset()
		if _, err := rolling.WriteWindow(&buf); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}

	rolling.Roll(byte('w'))
	window := readWindow()
	expected := "ello w"
	if string(window) != expected {
		t.Errorf("[%s] Expected the window to be '%s'", hashname, expected)
	}

	rolling.Roll(byte('o'))
	window = readWindow()
	expected = "llo wo"
	if string(window) != expected {
		t.Errorf("[%s] Expected the window to be '%s'", hashname, expected)
	}

	rolling.Roll(byte('r'))
	window = readWindow()
	expected = "lo wor"
	if string(window) != expected {
		t.Errorf("[%s] Expected the window to be '%s'", hashname, expected)
	}

	rolling.Roll(byte('l'))
	window = readWindow()
	expected = "o worl"
	if string(window) != expected {
		t.Errorf("[%s] Expected the window to be '%s'", hashname, expected)
	}

	rolling.Roll(byte('d'))
	window = readWindow()
	expected = " world"
	if string(window) != expected {
		t.Errorf("[%s] Expected the window to be '%s'", hashname, expected)
	}

	rolling.Roll(byte('!'))
	window = readWindow()
	expected = "world!"
	if string(window) != expected {
		t.Errorf("[%s] Expected the window to be '%s'", hashname, expected)
	}
}

func TestFoxDog(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		foxDog(t, h.name, h.classic, h.rolling)
	}
}

func TestRollEmptyWindow(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		rollEmptyWindow(t, h.name, h.classic, h.rolling)
	}
}

func TestWriteTwice(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		writeTwice(t, h.name, h.classic, h.rolling)
	}
}

func TestWriteTwiceThenRoll(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		writeTwiceThenRoll(t, h.name, h.classic, h.rolling)
	}
}

func TestWriteRollWrite(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		writeRollWrite(t, h.name, h.classic, h.rolling)
	}
}

func TestWriteThenWriteNothing(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		writeThenWriteNothing(t, h.name, h.classic, h.rolling)
	}
}

func TestWriteNothing(t *testing.T) {
	for _, h := range allHashes {
		h.classic.Reset()
		h.rolling.Reset()
		writeNothing(t, h.name, h.classic, h.rolling)
	}
}

func TestRead(t *testing.T) {
	for _, h := range allHashes {
		h.rolling.Reset()
		read(t, h.name, h.rolling)
	}
}

func TestBlockSize(t *testing.T) {
	for _, h := range allHashes {
		n := h.rolling.BlockSize()
		if n != 1 {
			t.Errorf("[%s] Expected BlockSize to return 1, got %v", h.name, n)
		}
	}
}

func TestSize(t *testing.T) {
	expectedSizes := map[string]int{
		"adler32":     4, // 32-bit hash
		"bozo32":      4, // 32-bit hash
		"bozo64":      8, // 64-bit hash
		"buzhash32":   4, // 32-bit hash
		"buzhash64":   8, // 64-bit hash
		"gearhash64":  8, // 64-bit hash
		"rabinkarp64": 8, // 64-bit hash
	}

	for _, h := range allHashes {
		expectedSize, exists := expectedSizes[h.name]
		if !exists {
			t.Errorf("[%s] No expected size defined for this hash", h.name)
			continue
		}

		actualSize := h.rolling.Size()
		if actualSize != expectedSize {
			t.Errorf("[%s] Expected Size to return %d, got %d", h.name, expectedSize, actualSize)
		}
	}
}

// FuzzRollingHashConsistency feeds random data and window sizes and
// verifies that rolling a window over the data always yields the same sum
// as hashing that window from scratch with the classic hash. It first
// checks the initial window (loaded with Write), then every subsequent
// window obtained by rolling one byte at a time.
func FuzzRollingHashConsistency(f *testing.F) {
	f.Add([]byte("hello world"), 5)
	f.Add([]byte("The quick brown fox jumps over the lazy dog"), 16)
	f.Add([]byte("a"), 1)
	f.Add([]byte(""), 0)
	f.Add([]byte("\x00\xff\xab\x01\x00\xfe\x42\x80"), 4)

	f.Fuzz(func(t *testing.T, data []byte, windowSize int) {
		if windowSize <= 0 || len(data) == 0 {
			return
		}
		if windowSize > len(data) {
			windowSize = len(data)
		}

		for _, h := range allHashes {
			classic := h.classic
			rolling := h.rolling

			classic.Reset()
			rolling.Reset()

			classic.Write(data[:windowSize])
			rolling.Write(data[:windowSize])

			expectedSum := sum64(classic)
			actualSum := sum64(rolling)

			if expectedSum != actualSum {
				t.Errorf("[%s] Initial hash mismatch: expected 0x%x, got 0x%x for data %q",
					h.name, expectedSum, actualSum, data[:windowSize])
			}

			for i := windowSize; i < len(data); i++ {
				classic.Reset()
				classic.Write(data[i-windowSize+1 : i+1])
				rolling.Roll(data[i])

				expectedSum = sum64(classic)
				actualSum = sum64(rolling)

				if expectedSum != actualSum {
					t.Errorf("[%s] Rolling hash mismatch at position %d: expected 0x%x, got 0x%x for window %q",
						h.name, i, expectedSum, actualSum, data[i-windowSize+1:i+1])
				}
			}
		}
	})
}

// batchRollOracle returns the expected BatchRoll output for data/window: the
// classic hash of every window-sized slice, read as a uint64.
func batchRollOracle(classic hash.Hash, data []byte, window int) []uint64 {
	if window <= 0 || len(data) < window {
		return nil
	}
	out := make([]uint64, len(data)-window+1)
	for i := range out {
		classic.Reset()
		classic.Write(data[i : i+window])
		out[i] = sum64(classic)
	}
	return out
}

// checkBatchRoll verifies a single bulk fast path implementation against the classic hash for a
// given data/window, including that dst has the expected length and that
// BatchRoll does not modify the receiver.
func checkBatchRoll(t *testing.T, name string, br batchRoller, classic hash.Hash, data []byte, window int) {
	t.Helper()
	want := batchRollOracle(classic, data, window)

	dst := make([]uint64, len(want))
	br.BatchRoll(dst, data, window)

	for i := range want {
		if dst[i] != want[i] {
			t.Errorf("[%s] BatchRoll(window=%d) at %d: expected 0x%x, got 0x%x (window %q)",
				name, window, i, want[i], dst[i], data[i:i+window])
		}
	}
}

// TestBatchRoll checks that every hash implementing the bulk fast path produces, for a
// range of inputs and window sizes, the same checksum at each position as the
// classic hash of that window. The edge cases target the two-lane split:
// empty output, a single output (no second lane), window==1, and both odd and
// even output counts so the unequal-lane tail is exercised.
func TestBatchRoll(t *testing.T) {
	base := []byte("The quick brown fox jumps over the lazy dog")
	for _, h := range allHashes {
		br, ok := h.rolling.(batchRoller)
		if !ok {
			continue
		}
		// data, window pairs. Output count is len(data)-window+1.
		cases := []struct {
			data   []byte
			window int
		}{
			{base, 1},             // window==1
			{base, 16},            // typical
			{base, len(base)},     // exactly one output, no lane B
			{base, len(base) + 1}, // window > len: empty output
			{base[:5], 4},         // 2 outputs (even)
			{base[:6], 4},         // 3 outputs (odd)
			{base[:7], 4},         // 4 outputs (even)
			{base[:8], 4},         // 5 outputs (odd)
			{[]byte("a"), 1},      // single byte
			{[]byte{}, 1},         // empty data
		}
		for _, c := range cases {
			checkBatchRoll(t, h.name, br, h.classic, c.data, c.window)
		}
	}
}

// FuzzBatchRoll feeds random data and window sizes and verifies that, for
// every hash implementing the bulk fast path, the bulk output matches the classic
// hash of each window position.
func FuzzBatchRoll(f *testing.F) {
	f.Add([]byte("hello world"), 5)
	f.Add([]byte("The quick brown fox jumps over the lazy dog"), 16)
	f.Add([]byte("a"), 1)
	f.Add([]byte(""), 0)

	f.Fuzz(func(t *testing.T, data []byte, windowSize int) {
		if windowSize <= 0 || len(data) == 0 {
			return
		}
		if windowSize > len(data) {
			windowSize = len(data)
		}

		for _, h := range allHashes {
			br, ok := h.rolling.(batchRoller)
			if !ok {
				continue
			}
			checkBatchRoll(t, h.name, br, h.classic, data, windowSize)
		}
	})
}

// checkBatchBoundaries verifies the boundary fast path against the classic hash: the
// reported positions (lane a[:na] followed by lane b[:nb]) must be exactly the
// ascending set {i : classic(data[i:i+window]) & mask == 0}.
func checkBatchBoundaries(t *testing.T, name string, brd boundaryRoller, classic hash.Hash, data []byte, window int, mask uint64) {
	t.Helper()
	sums := batchRollOracle(classic, data, window)
	var want []int32
	for i, s := range sums {
		if s&mask == 0 {
			want = append(want, int32(i))
		}
	}

	a := make([]int32, len(sums))
	b := make([]int32, len(sums))
	na, nb := brd.BatchBoundaries(a, b, data, window, mask)
	got := append(append([]int32(nil), a[:na]...), b[:nb]...)

	if len(got) != len(want) {
		t.Errorf("[%s] BatchBoundaries(window=%d,mask=0x%x): got %d hits, want %d",
			name, window, mask, len(got), len(want))
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%s] BatchBoundaries hit %d: got %d, want %d", name, i, got[i], want[i])
		}
	}
}

// TestBatchBoundaries checks that every hash implementing the boundary fast path reports
// exactly the window positions whose classic checksum satisfies sum&mask==0,
// across windows and masks (including the degenerate mask==0, which matches
// every position and so stresses the lane buffers and the A/B merge order).
func TestBatchBoundaries(t *testing.T) {
	base := []byte("The quick brown fox jumps over the lazy dog")
	for _, h := range allHashes {
		brd, ok := h.rolling.(boundaryRoller)
		if !ok {
			continue
		}
		cases := []struct {
			data   []byte
			window int
		}{
			{base, 1}, {base, 8}, {base, 16},
			{base, len(base)}, {base[:5], 4}, {base[:6], 4}, {base[:8], 4},
		}
		for _, c := range cases {
			for _, mask := range []uint64{0, 1, 0x3, 0xff, 0xffffffffffffffff} {
				checkBatchBoundaries(t, h.name, brd, h.classic, c.data, c.window, mask)
			}
		}
	}
}

// FuzzBatchBoundaries cross-checks BatchBoundaries against the classic hash on
// random data, window, and mask.
func FuzzBatchBoundaries(f *testing.F) {
	f.Add([]byte("The quick brown fox jumps over the lazy dog"), 16, uint64(0x3))
	f.Add([]byte("hello world"), 5, uint64(1))
	f.Add([]byte("a"), 1, uint64(0))

	f.Fuzz(func(t *testing.T, data []byte, windowSize int, mask uint64) {
		if windowSize <= 0 || len(data) == 0 {
			return
		}
		if windowSize > len(data) {
			windowSize = len(data)
		}
		for _, h := range allHashes {
			brd, ok := h.rolling.(boundaryRoller)
			if !ok {
				continue
			}
			checkBatchBoundaries(t, h.name, brd, h.classic, data, windowSize, mask)
		}
	})
}

// FuzzWriteConsistency checks two write-related invariants on random
// inputs. First, that splitting the input across two Write calls produces
// the same sum as a single Write of the concatenation. Second, that
// writing all but the last byte and then rolling the last byte (the
// write-and-roll pattern) matches the classic hash of the full input.
func FuzzWriteConsistency(f *testing.F) {
	f.Add([]byte("hello"), []byte("world"))
	f.Add([]byte("test"), []byte("data"))
	f.Add([]byte("a"), []byte("b"))
	f.Add([]byte(""), []byte("x"))

	f.Fuzz(func(t *testing.T, part1, part2 []byte) {
		if len(part1) == 0 && len(part2) == 0 {
			return
		}

		for _, h := range allHashes {
			classic := h.classic
			rolling := h.rolling

			classic.Reset()
			rolling.Reset()

			rolling.Write(part1)
			rolling.Write(part2)

			fullData := append(part1, part2...)
			classic.Write(fullData)

			expectedSum := sum64(classic)
			actualSum := sum64(rolling)

			if expectedSum != actualSum {
				t.Errorf("[%s] Write sequence mismatch: expected 0x%x, got 0x%x for parts %q + %q",
					h.name, expectedSum, actualSum, part1, part2)
			}

			if len(fullData) > 0 {
				classic.Reset()
				rolling.Reset()

				padded := append([]byte{0}, fullData...)
				rolling.Write(padded[:len(padded)-1])
				rolling.Roll(padded[len(padded)-1])

				classic.Write(fullData)

				expectedSum = sum64(classic)
				actualSum = sum64(rolling)

				if expectedSum != actualSum {
					t.Errorf("[%s] Write+Roll mismatch: expected 0x%x, got 0x%x for data %q",
						h.name, expectedSum, actualSum, fullData)
				}
			}
		}
	})
}

// FuzzWindowReading verifies that WriteWindow always reflects the exact
// bytes currently inside the rolling window. It loads an initial window,
// checks the reported byte count matches the data written, then rolls a
// few bytes and confirms the window content tracks the sliding view of the
// input after each roll.
func FuzzWindowReading(f *testing.F) {
	f.Add([]byte("hello world test"))
	f.Add([]byte("abcdefgh"))
	f.Add([]byte("The quick brown fox jumps"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// An empty input has no valid window to load (a window must be at
		// least 1 byte), so there is nothing to check.
		if len(data) == 0 {
			return
		}

		for _, h := range allHashes {
			rolling := h.rolling
			rolling.Reset()

			// len(data) == 1 yields windowSize 0, so clamp to a minimal
			// 1-byte window; the roll loop below then simply does not run.
			windowSize := len(data) / 2
			if windowSize == 0 {
				windowSize = 1
			}

			rolling.Write(data[:windowSize])

			var buf bytes.Buffer
			n, err := rolling.WriteWindow(&buf)
			if err != nil {
				t.Errorf("[%s] WriteWindow failed: %v", h.name, err)
				continue
			}

			window := buf.Bytes()
			if len(window) != n {
				t.Errorf("[%s] WriteWindow returned length %d but wrote %d bytes", h.name, n, len(window))
			}

			if !bytes.Equal(window, data[:windowSize]) {
				t.Errorf("[%s] Initial window mismatch: expected %q, got %q", h.name, data[:windowSize], window)
			}

			for i := windowSize; i < len(data) && i < windowSize+10; i++ {
				rolling.Roll(data[i])

				buf.Reset()
				_, err = rolling.WriteWindow(&buf)
				if err != nil {
					t.Errorf("[%s] WriteWindow after roll failed: %v", h.name, err)
					continue
				}

				window = buf.Bytes()
				expected := data[i-windowSize+1 : i+1]

				if !bytes.Equal(window, expected) {
					t.Errorf("[%s] Window after roll %d mismatch: expected %q, got %q",
						h.name, i-windowSize+1, expected, window)
				}
			}
		}
	})
}

// FuzzEdgeCases exercises boundary behaviours on random inputs: writing an
// empty slice, writing an empty slice after real data (which must not
// corrupt the window), rolling past the end of the window, and the
// invariant BlockSize and Size values. It is meant to surface crashes and
// state corruption rather than to compare against the classic hash.
func FuzzEdgeCases(f *testing.F) {
	f.Add([]byte("x"))
	f.Add([]byte("ab"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		for _, h := range allHashes {
			rolling := h.rolling

			rolling.Reset()
			rolling.Write([]byte(""))

			rolling.Reset()
			if len(data) > 0 {
				rolling.Write(data)

				rolling.Write([]byte(""))

				var buf bytes.Buffer
				_, err := rolling.WriteWindow(&buf)
				if err != nil {
					t.Errorf("[%s] WriteWindow after empty write failed: %v", h.name, err)
				}

				if !bytes.Equal(buf.Bytes(), data) {
					t.Errorf("[%s] Window corrupted after empty write: expected %q, got %q",
						h.name, data, buf.Bytes())
				}

				if len(data) > 1 {
					rolling.Reset()
					rolling.Write(data[:len(data)-1])
					rolling.Roll(data[len(data)-1])
					rolling.Roll('x')
				}
			}

			rolling.Reset()
			if rolling.BlockSize() != 1 {
				t.Errorf("[%s] BlockSize should always be 1, got %d", h.name, rolling.BlockSize())
			}

			expectedSize := map[string]int{
				"adler32": 4, "bozo32": 4, "buzhash32": 4,
				"bozo64": 8, "buzhash64": 8, "gearhash64": 8, "rabinkarp64": 8,
			}
			if size, ok := expectedSize[h.name]; ok && rolling.Size() != size {
				t.Errorf("[%s] Size should be %d, got %d", h.name, size, rolling.Size())
			}
		}
	})
}
