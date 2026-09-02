// Package restic is a drop-in replacement for the consumer API of
// github.com/restic/chunker. Its New/NewWithBoundaries, *Chunker, Chunk, Pol,
// RandomPolynomial/DerivePolynomial, MinSize/MaxSize, the WithBoundaries /
// WithAverageBits / WithBuffer options, and the Next/Reset/SetAverageBits
// methods match that package's signatures, and it produces byte-identical chunk
// boundaries. Migrating is a one-line change:
//
//	import chunker "github.com/chmduquesne/rollinghash/v4/cdc/compat/restic"
//
// # How it works
//
// restic/chunker's splitter is a Rabin fingerprint over GF(2)[X] modulo a
// degree-53 irreducible polynomial, with a 64-byte sliding window, a single
// splitmask of (1<<averageBits)-1, and chunk lengths clamped to [MinSize,
// MaxSize]. This repo already ships that exact rolling hash: rabinkarp64 was
// adapted from restic/chunker itself (same polynomial arithmetic, same out/mod
// tables). This package pairs it with rollinghash.NewChunker, which is a
// single-mask, rolling-window, [min,max]-clamped CDC engine. The pre-min skip
// of both implementations lines up, so the fingerprint tested at every
// candidate boundary is identical.
//
// # Differences from restic/chunker
//
//   - Chunk.Cut agrees with restic at every real content-defined boundary and at
//     a forced cut at MaxSize. It differs only on a final chunk shorter than
//     MinSize: restic never fed the rolling window there, so it returns an
//     uninitialised digest its own source calls "somewhat meaningless as this is
//     not a split point"; this package returns the real fingerprint of the last
//     64 bytes, or 0 when the whole stream is shorter than 64 bytes.
//   - MinSize must be at least 64 (the window). restic's default is 512 KiB and
//     a smaller-than-window MinSize misbehaves there too (unsigned underflow);
//     this package does not validate it.
//   - The BaseChunker / NewBase API (chunking without an io.Reader) is not
//     mirrored.
//
// Callers who want the idiomatic API should use rabinkarp64.NewFromPol together
// with rollinghash.NewChunker directly.
package restic
