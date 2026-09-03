// Package duplicacy reproduces the content-defined chunk boundaries of
// github.com/gilbertchen/duplicacy's ChunkMaker. Given the same chunk seed and
// the same average/minimum/maximum size triple it splits a byte stream at
// exactly the same offsets, so a repository chunked here indexes identically to
// one chunked by Duplicacy itself.
//
//	import chunker "github.com/chmduquesne/rollinghash/v4/cdc/compat/duplicacy"
//
//	m := chunker.CreateChunkMaker() // defaults: seed "duplicacy", 4/1/16 MiB
//	m.AddData(reader, func(c chunker.Chunk) { store(c.Data) })
//	m.AddData(nil, func(c chunker.Chunk) { store(c.Data) }) // flush the tail
//
// # How it works
//
// Duplicacy's splitter is a buzhash (cyclic polynomial) rolling hash whose
// window length equals the minimum chunk size, with a single boundary mask of
// (averageChunkSize-1) and chunk lengths clamped to [minimumChunkSize,
// maximumChunkSize]. The 256-entry byte table is derived from the repository's
// ChunkSeed by a chain of SHA-256 digests. This package rebuilds that table
// (see deriveTable) and pairs it with buzhash64 and
// rollinghash.NewChunkWriter, this module's single-mask, rolling-window,
// [min,max]-clamped CDC engine. Duplicacy computes the hash of the first
// minimumChunkSize bytes of every chunk and only then starts testing the mask,
// one byte at a time; NewChunkWriter's pre-min skip lines up with that, so the
// window tested at every candidate boundary is identical.
//
// # Performance
//
// The buzhash window is the full minimumChunkSize (a megabyte with the default
// sizes), where restic's is 64 bytes. NewChunkWriter's BatchBoundaries re-primes
// that window on every batch, so CreateChunkMaker defaults the batch to roughly
// eight windows (capped at 16 MiB) to amortise it; that buffer is allocated once
// per maker and kept across Reset. Even so it runs at about 0.85x of a
// continuous single-pass roll of the same algorithm — the per-batch re-prime
// and the per-cut window recompute for Chunk.Hash are work that roll never does.
// BenchmarkChunkMaker measures both side by side (Duplicacy ships no consumable
// Go module for a real head-to-head, so its "reference" arm is an independent
// single-pass implementation of the algorithm). The multiple-of-64 window also
// rules out any SIMD, so this is slower in absolute terms than the small-window
// CDC algorithms in this module. In a real backup, chunk hashing and I/O
// dominate and the splitter is not the bottleneck.
//
// Because Duplicacy requires averageChunkSize to be a power of two, the buzhash
// window (= minimumChunkSize, itself normally a power of two) is a multiple of
// 64. That is the degenerate case called out in the buzhash64 package doc: the
// leaving-byte rotation collapses to no rotation at all. This package
// faithfully reproduces that behaviour — it is what Duplicacy does — but it
// means the boundary distribution carries buzhash64's known bias for such
// windows. It matches Duplicacy regardless.
//
// # Differences from github.com/gilbertchen/duplicacy
//
//   - Chunk boundaries and the buzhash at each boundary (Chunk.Hash — the value
//     Duplicacy tests against its hashMask) are reproduced. Duplicacy's Chunk
//     also carries the chunk's *content* hash and, from it, the chunk ID and
//     file name; those depend on the repository's HashKey and compression
//     settings and are out of scope here.
//   - Duplicacy forces a cut at maximumChunkSize cleanly only when
//     maximumChunkSize is a multiple of minimumChunkSize (true for every size
//     triple its CLI produces, where the minimum is a quarter of the average and
//     the maximum four times it). This package always forces the cut at exactly
//     maximumChunkSize; for an unusual triple where the maximum is not a
//     multiple of the minimum the two can therefore disagree around a forced
//     cut.
//   - The fixed-size path (minimumChunkSize == maximumChunkSize) and the
//     metadata chunk maker are not mirrored as separate entry points; passing an
//     equal minimum and maximum to CreateChunkMaker still produces fixed-size
//     chunks.
//   - Verified against an independent reimplementation of Duplicacy's algorithm
//     (Duplicacy ships no consumable Go module: its pre-go.mod tags no longer
//     build against current transitive dependencies and its v3 tags are
//     rejected by the module system for lacking a /v3 path). See
//     duplicacy_test.go.
//
// Callers who want the idiomatic API should use buzhash64.NewFromUint64Array
// together with rollinghash.NewChunkWriter (or NewChunker) directly.
package duplicacy
