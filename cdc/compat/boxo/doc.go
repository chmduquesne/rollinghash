// Package boxo is a drop-in replacement for github.com/ipfs/boxo/chunker
// (package chunk). Its Splitter interface, NewSizeSplitter / NewRabin /
// NewRabinMinMax / NewBuzhash constructors, DefaultSplitter, SizeSplitterGen,
// Chan, FromString and Register all match that package's signatures, and it
// produces byte-identical chunk boundaries. The CIDs of content chunked through
// it are therefore identical to boxo's.
//
// # Migrating
//
// The replacement is at the interface level. Values returned by this package
// (the Splitter constructors, FromString, DefaultSplitter) satisfy
// boxo/chunker.Splitter structurally, so a call site that consumes a Splitter
// migrates with a one-line import swap:
//
//	import chunk "github.com/chmduquesne/rollinghash/v4/cdc/compat/boxo"
//
// A call site that passes this package's named SplitterGen or SplitterFunc type
// into a boxo function needs more than an import swap: boxo/chunker.SplitterGen
// and this package's SplitterGen are distinct named types with distinct
// underlying types, so Go will not assign one to the other. Keep the
// boxo/chunker import for the named type and route only the rolling-hash
// construction through this package, wrapping it in a closure whose declared
// return type is boxo's Splitter interface:
//
//	return func(r io.Reader) chunk.Splitter {
//		s, err := rollinghash.FromString(r, chunkerStr)
//		...
//	}
//
// Kubo's config/import.go is the one call site in the IPFS tree that hits this;
// its ipfs add path (core/coreunix) migrates with the bare import swap.
//
// # How it works
//
// boxo's three splitters map onto this module's engine:
//
//   - "rabin" is github.com/whyrusleeping/chunker, an unmaintained restic/chunker
//     fork: a Rabin fingerprint over the degree-53 polynomial IpfsRabinPoly, a
//     16-byte window, and a mask of (1<<floor(log2(avg)))-1. Reproduced with
//     rabinkarp64 (itself a maintained descendant of the same restic/chunker
//     lineage) plus rollinghash.NewChunker.
//   - "buzhash" is a 32-bit cyclic-polynomial hash with a fixed 256-entry table,
//     a 32-byte window, a 17-bit mask and fixed 128 KiB–512 KiB bounds.
//     Reproduced with buzhash32 (loaded with boxo's table) plus
//     rollinghash.NewChunker. A 32-byte window equals the word size, which the
//     buzhash32 docs warn against in general, but there the rolling recurrence
//     collapses to boxo's exact update, so the two agree.
//   - "size" is fixed-size splitting and uses no rolling hash.
//
// Byte-for-byte agreement with the real package is verified by the
// cdc/compat/boxo/bench nested module, which imports github.com/ipfs/boxo
// directly as the oracle.
//
// # Differences from boxo/chunker
//
//   - Each NextBytes result is a freshly allocated, caller-owned slice, as in
//     boxo. Callers that want zero-copy iteration should use
//     rollinghash.NewChunker directly. The internal accumulator buffer is
//     pooled across Splitter instances (like boxo's go-buffer-pool use), so
//     creating one Splitter per stream does not re-grow a buffer each time.
//   - Registering a custom chunker with Register affects only this package's
//     registry, not boxo's, and vice versa.
//
// Callers who want the idiomatic API should use rabinkarp64 / buzhash32 with
// rollinghash.NewChunker directly.
package boxo
