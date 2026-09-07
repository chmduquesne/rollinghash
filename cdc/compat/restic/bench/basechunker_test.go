package bench

import (
	"testing"

	compat "github.com/chmduquesne/rollinghash/v4/cdc/compat/restic"
	rc "github.com/restic/chunker"
)

// baseSplitter is the one method restic drives its file chunking through
// (internal/repository/chunker.go). Both the real BaseChunker and the compat
// one implement it.
type baseSplitter interface {
	NextSplitPoint(buf []byte) int
}

// splitLengths chunks data exactly the way restic's archiver does
// (internal/archiver/file_saver.go readNextChunk): read the input in blockSize
// slices, and within each slice call NextSplitPoint on the not-yet-consumed
// tail until it reports no boundary. The trailing bytes form a final chunk, as
// restic's caller does on io.EOF. It returns the chunk lengths.
func splitLengths(bc baseSplitter, data []byte, blockSize int) []int {
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

// TestParityBaseChunker is the byte-identical proof for the incremental API:
// for a grid of polynomials, bounds, average-bits, data shapes and read-block
// sizes, cdc/compat/restic.BaseChunker must cut at exactly the same offsets as
// the real github.com/restic/chunker.BaseChunker.
func TestParityBaseChunker(t *testing.T) {
	pols := []rc.Pol{resticDefaultPol}
	if p, err := rc.RandomPolynomial(); err == nil {
		pols = append(pols, p)
	}

	shapes := map[string]func(int) []byte{"rand": randData, "struct": structData}
	sizes := []int{0, 1, 63, 64, 200, 5000, 70_000, 300_000, 1 << 20, 3 << 20}
	configs := []struct {
		min, max uint
		bits     int
	}{
		{64, 512, 8},
		{512, 4096, 11},
		{1024, 65536, 14},
		{2048, 32768, 13},
	}
	blockSizes := []int{64, 1000, 65536, 512 << 10}

	for _, pol := range pols {
		for shapeName, gen := range shapes {
			for _, n := range sizes {
				data := gen(n)
				for _, cfg := range configs {
					for _, bs := range blockSizes {
						real := rc.NewBase(pol,
							rc.WithBaseBoundaries(cfg.min, cfg.max),
							rc.WithBaseAverageBits(cfg.bits))
						ours := compat.NewBase(compat.Pol(pol),
							compat.WithBaseBoundaries(cfg.min, cfg.max),
							compat.WithBaseAverageBits(cfg.bits))

						want := splitLengths(real, data, bs)
						got := splitLengths(ours, data, bs)

						if !equalInts(want, got) {
							t.Fatalf("pol=%#x %s n=%d cfg=%v block=%d:\n real: %v\n ours: %v",
								uint64(pol), shapeName, n, cfg, bs, want, got)
						}
					}
				}
			}
		}
	}
}

// TestParityBaseChunkerReset checks that Reset returns a BaseChunker to a
// pristine state, matching the real one across two passes over the same data.
func TestParityBaseChunkerReset(t *testing.T) {
	data := structData(2 << 20)

	real := rc.NewBase(resticDefaultPol)
	ours := compat.NewBase(compat.Pol(resticDefaultPol))

	for pass := range 2 {
		want := splitLengths(real, data, 128<<10)
		got := splitLengths(ours, data, 128<<10)
		if !equalInts(want, got) {
			t.Fatalf("pass %d:\n real: %v\n ours: %v", pass, want, got)
		}
		real.Reset(resticDefaultPol)
		ours.Reset(compat.Pol(resticDefaultPol))
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

// drainBase feeds data through bc exactly as restic's archiver does
// (internal/archiver/file_saver.go readNextChunk): 512 KiB read blocks, with
// NextSplitPoint called on the unconsumed tail of each block until it reports
// no boundary.
func drainBase(bc baseSplitter, data []byte) {
	const blockSize = 512 << 10 // restic's chunkReadBufSize
	for pos := 0; pos < len(data); {
		end := min(pos+blockSize, len(data))
		block := data[pos:end]
		for bpos := 0; bpos < len(block); {
			if split := bc.NextSplitPoint(block[bpos:]); split < 0 {
				bpos = len(block)
			} else {
				bpos += split
			}
		}
		pos = end
	}
}

// BenchmarkRestic_Real and BenchmarkRestic_Compat are the head-to-head on the
// API restic actually chunks file data with: a BaseChunker fed 512 KiB blocks
// through NextSplitPoint, kept across files and re-armed with Reset (restic's
// archiver holds one chunker per worker and Resets it per file). Each iteration
// chunks one file's worth of data. The chunker is warmed once before the timer
// so the numbers are the steady state, not the first file's buffer growth.
func BenchmarkRestic_Real(b *testing.B) {
	data := benchData()
	b.SetBytes(int64(len(data)))
	c := rc.NewBase(resticDefaultPol)
	drainBase(c, data)
	b.ResetTimer()
	for range b.N {
		c.Reset(resticDefaultPol)
		drainBase(c, data)
	}
}

func BenchmarkRestic_Compat(b *testing.B) {
	data := benchData()
	b.SetBytes(int64(len(data)))
	c := compat.NewBase(compat.Pol(resticDefaultPol))
	drainBase(c, data)
	b.ResetTimer()
	for range b.N {
		c.Reset(compat.Pol(resticDefaultPol))
		drainBase(c, data)
	}
}
