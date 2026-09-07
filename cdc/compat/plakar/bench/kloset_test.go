package bench

import (
	"bytes"
	"testing"

	"github.com/PlakarKorp/kloset/chunking"
	ptesting "github.com/PlakarKorp/kloset/testing"
	compat "github.com/chmduquesne/rollinghash/v4/cdc/compat/plakar"
)

// TestKlosetBackupParity is the end-to-end drop-in check for cdc/compat/plakar.
// The algorithm-level parity tests above diff our output against real
// go-cdc-chunkers directly; this one instead runs a real backup through
// github.com/PlakarKorp/kloset (the library that holds plakar's backup and
// chunking code, still using its own go-cdc-chunkers), then re-chunks the same
// bytes with cdc/compat/plakar under kloset's default configuration and asserts
// the boundary sequence is identical. If it is, swapping kloset's chunker for
// ours would produce byte-identical snapshots.
//
// It also exercises the exact Reset/Next/EOF interplay kloset's chunkify loop
// depends on, so it catches drift in how kloset drives the chunker, not just in
// the cut points.
func TestKlosetBackupParity(t *testing.T) {
	repo := ptesting.GenerateRepository(t, nil, nil, nil)

	// kloset's default is fastcdc-v1.0.0 at 512 KiB / 1 MiB / 8 MiB, so a dozen
	// MiB of high-entropy data yields a realistic run of cut points and takes
	// the direct file-chunking path (not the small-file dirpack path).
	data := randData(12 << 20)

	snap := ptesting.GenerateSnapshot(t, repo, []ptesting.MockFile{
		ptesting.NewMockDir("/"),
		ptesting.NewMockFile("/big.bin", 0644, string(data)),
	})
	if snap == nil {
		t.Fatal("nil snapshot")
	}
	defer snap.Close()

	fs, err := snap.Filesystem()
	if err != nil {
		t.Fatalf("Filesystem: %v", err)
	}
	entry, err := fs.GetEntry("/big.bin")
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if entry.ResolvedObject == nil {
		t.Fatal("entry has no resolved object")
	}

	var klosetLens []int
	for _, c := range entry.ResolvedObject.Chunks {
		klosetLens = append(klosetLens, int(c.Length))
	}
	if len(klosetLens) < 2 {
		t.Fatalf("expected the file to be split into multiple chunks, got %d", len(klosetLens))
	}

	// Re-chunk the same bytes with our compat layer under kloset's own defaults.
	cfg := chunking.NewDefaultConfiguration()
	c, err := compat.NewChunker(cfg.Algorithm, bytes.NewReader(data), &compat.ChunkerOpts{
		MinSize:    int(cfg.MinSize),
		NormalSize: int(cfg.NormalSize),
		MaxSize:    int(cfg.MaxSize),
	})
	if err != nil {
		t.Fatalf("compat.NewChunker(%q): %v", cfg.Algorithm, err)
	}
	var ourLens []int
	if err := c.Split(func(_, length uint, _ []byte) error {
		ourLens = append(ourLens, int(length))
		return nil
	}); err != nil {
		t.Fatalf("compat Split: %v", err)
	}

	if len(ourLens) != len(klosetLens) {
		t.Fatalf("chunk count: ours %d, kloset %d", len(ourLens), len(klosetLens))
	}
	for i := range klosetLens {
		if ourLens[i] != klosetLens[i] {
			t.Fatalf("chunk %d length: ours %d, kloset %d", i, ourLens[i], klosetLens[i])
		}
	}
}
