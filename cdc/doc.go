/*
Package cdc is an umbrella for content-defined chunking (CDC) algorithms
built on the rolling hashes in the parent module.

Each algorithm lives in its own subpackage:

  - [github.com/chmduquesne/rollinghash/v4/cdc/fastcdc]: FastCDC (normalized
    Gear chunking with cut-point skipping)
  - [github.com/chmduquesne/rollinghash/v4/cdc/jumpchunker]: Jump Chunking (JC)
  - [github.com/chmduquesne/rollinghash/v4/cdc/ultracdc]: UltraCDC (8-byte
    window, Hamming distance to 0xAA, no rolling hash)
  - [github.com/chmduquesne/rollinghash/v4/cdc/maxcdc]: MaxCDC (cut before the
    Gear-fingerprint maximum in [minSize, maxSize]), matching buildbarn/go-cdc
  - [github.com/chmduquesne/rollinghash/v4/cdc/repmaxcdc]: RepMaxCDC (repeated
    Gear-fingerprint maximum, strict [minSize, 2*minSize) chunk sizes), matching
    buildbarn/go-cdc
  - [github.com/chmduquesne/rollinghash/v4/cdc/repmaxsfxcdc]: RepMaxSfxCDC
    (RepMaxCDC with a lexicographic-suffix extremum instead of a Gear
    fingerprint), matching buildbarn/go-cdc
  - [github.com/chmduquesne/rollinghash/v4/cdc/aecdc]: AE (Asymmetric Extremum;
    raw-byte extremum, unbounded left window, no rolling hash), matching
    buildbarn/go-cdc

Every *Chunker implements [github.com/chmduquesne/rollinghash/v4.Chunker] and
every *ChunkWriter (from each package's NewChunkWriter, the push-based
counterpart fed via Write/Close) implements
[github.com/chmduquesne/rollinghash/v4.ChunkWriter], so one loop works across
all algorithms and both drive styles. Boundaries are byte-for-byte compatible
with the corresponding algorithm in PlakarKorp/go-cdc-chunkers;
[github.com/chmduquesne/rollinghash/v4/cdc/compat/plakar] is a signature-compatible
drop-in for that library's NewChunker/Next/Split/Copy API.
[github.com/chmduquesne/rollinghash/v4/cdc/compat/restic] does the same for
github.com/restic/chunker's Rabin fingerprint splitter, producing byte-identical
boundaries via the rabinkarp64 hash.
[github.com/chmduquesne/rollinghash/v4/cdc/compat/boxo] does the same for
github.com/ipfs/boxo/chunker (rabin, buzhash and fixed-size splitting plus its
FromString registry), producing byte-identical chunks — and therefore identical
IPFS CIDs.
[github.com/chmduquesne/rollinghash/v4/cdc/compat/duplicacy] reproduces the
chunk boundaries of github.com/gilbertchen/duplicacy's ChunkMaker (buzhash with
a window equal to the minimum chunk size, seeded from the repository ChunkSeed).
[github.com/chmduquesne/rollinghash/v4/cdc/maxcdc],
[github.com/chmduquesne/rollinghash/v4/cdc/repmaxcdc],
[github.com/chmduquesne/rollinghash/v4/cdc/repmaxsfxcdc] and
[github.com/chmduquesne/rollinghash/v4/cdc/aecdc] match
github.com/buildbarn/go-cdc's MaxCDC, RepMaxCDC, RepMaxSfxCDC and AE for the
same size parameters (and, for the Gear chunkers, the same Gear table);
[github.com/chmduquesne/rollinghash/v4/cdc/compat/buildbarn] is a
signature-compatible drop-in for that library's ContentDefinedChunker /
ChunkReader / Peeker consumer API, covering those four plus FastCDC (via
cdc/fastcdc's WithInclusiveBoundary), verified byte-for-byte against the real
package by the cdc/compat/buildbarn/bench nested module.

The parent package's [github.com/chmduquesne/rollinghash/v4.Chunker] and
[github.com/chmduquesne/rollinghash/v4.ChunkWriter] provide the classic
single-mask, rolling-window CDC that works with any hash implementing
BatchBoundaries. The subpackages here cover algorithms that need their own
boundary logic or their own hash.
*/
package cdc
