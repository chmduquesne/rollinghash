/*
Package cdc is an umbrella for content-defined chunking (CDC) algorithms
built on the rolling hashes in the parent module.

Each algorithm lives in its own subpackage:

  - [github.com/chmduquesne/rollinghash/v4/cdc/fastcdc]: FastCDC (normalized
    Gear chunking with cut-point skipping)
  - [github.com/chmduquesne/rollinghash/v4/cdc/jumpchunker]: Jump Chunking (JC)
  - [github.com/chmduquesne/rollinghash/v4/cdc/ultracdc]: UltraCDC (8-byte
    window, Hamming distance to 0xAA, no rolling hash)

Every *Chunker implements [github.com/chmduquesne/rollinghash/v4.Chunker] and
every *ChunkWriter (from each package's NewChunkWriter, the push-based
counterpart fed via Write/Close) implements
[github.com/chmduquesne/rollinghash/v4.ChunkWriter], so one loop works across
all algorithms and both drive styles. Boundaries are byte-for-byte compatible
with the corresponding algorithm in PlakarKorp/go-cdc-chunkers;
[github.com/chmduquesne/rollinghash/v4/cdc/gocdc] is a signature-compatible
drop-in for that library's NewChunker/Next/Split/Copy API.

The parent package's [github.com/chmduquesne/rollinghash/v4.Chunker] and
[github.com/chmduquesne/rollinghash/v4.ChunkWriter] provide the classic
single-mask, rolling-window CDC that works with any hash implementing
BatchBoundaries. The subpackages here cover algorithms that need their own
boundary logic or their own hash.
*/
package cdc
