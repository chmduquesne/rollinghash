package restic_test

import (
	"testing"

	restic "github.com/chmduquesne/rollinghash/v4/cdc/compat/restic"
)

// baseSplitLengths chunks data the way restic's archiver drives a BaseChunker
// (internal/archiver/file_saver.go): read the input in blockSize slices and
// call NextSplitPoint on the unconsumed tail until it reports no boundary; the
// trailing bytes form a final chunk. It returns the chunk lengths.
func baseSplitLengths(bc *restic.BaseChunker, data []byte, blockSize int) []int {
	var lengths []int
	cur := 0
	for pos := 0; pos < len(data); {
		end := min(pos+blockSize, len(data))
		block := data[pos:end]
		for bpos := 0; bpos < len(block); {
			split := bc.NextSplitPoint(block[bpos:])
			if split < 0 {
				cur += len(block) - bpos
				bpos = len(block)
			} else {
				cur += split
				bpos += split
				lengths = append(lengths, cur)
				cur = 0
			}
		}
		pos = end
	}
	if cur > 0 {
		lengths = append(lengths, cur)
	}
	return lengths
}

// TestGoldenBaseChunker pins the exact BaseChunker boundaries for a fixed
// polynomial, data and options. The lengths were produced by the real
// github.com/restic/chunker v0.5.0 BaseChunker and verified byte-identical (see
// cdc/compat/restic/bench). They also match TestGoldenBoundaries, the
// reader-API golden for the same input. This is the no-external-dependency
// regression guard.
func TestGoldenBaseChunker(t *testing.T) {
	want := []int{735, 1612, 5061, 811, 2497, 2306, 3755, 3395, 347, 1999,
		3915, 1480, 833, 275, 593, 3917, 262, 286, 360, 3351, 4992, 1430,
		3446, 1683, 659}

	data := xorshiftData(50000)

	// The read-block size must not change the boundaries.
	for _, bs := range []int{64, 999, 4096, len(data)} {
		bc := restic.NewBase(resticDefaultPol,
			restic.WithBaseBoundaries(256, 8192), restic.WithBaseAverageBits(11))
		got := baseSplitLengths(bc, data, bs)

		if len(got) != len(want) {
			t.Fatalf("block=%d: got %d chunks, want %d (%v)", bs, len(got), len(want), got)
		}
		var total int
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("block=%d chunk %d: length %d, want %d", bs, i, got[i], want[i])
			}
			total += got[i]
		}
		if total != len(data) {
			t.Errorf("block=%d: chunks cover %d bytes, want %d", bs, total, len(data))
		}
	}
}

func TestBaseChunkerResetIsPristine(t *testing.T) {
	data := xorshiftData(60000)
	bc := restic.NewBase(resticDefaultPol,
		restic.WithBaseBoundaries(256, 8192), restic.WithBaseAverageBits(11))

	first := baseSplitLengths(bc, data, 4096)
	bc.Reset(resticDefaultPol,
		restic.WithBaseBoundaries(256, 8192), restic.WithBaseAverageBits(11))
	second := baseSplitLengths(bc, data, 4096)

	if len(first) != len(second) {
		t.Fatalf("after Reset: %d chunks, first pass had %d", len(second), len(first))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("chunk %d differs after Reset: %d vs %d", i, first[i], second[i])
		}
	}
}

func TestNewBasePanicsOnBadPolynomial(t *testing.T) {
	for _, pol := range []restic.Pol{0, 1 << 54} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewBase(pol=%#x) did not panic", uint64(pol))
				}
			}()
			restic.NewBase(pol)
		}()
	}
}
