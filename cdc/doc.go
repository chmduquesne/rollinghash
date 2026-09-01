/*
Package cdc is an umbrella for content-defined chunking (CDC) algorithms
built on the rolling hashes in the parent module.

Each algorithm lives in its own subpackage. Currently:

  - [github.com/chmduquesne/rollinghash/v4/cdc/jumpchunker]: Jump Chunking (JC)

FastCDC and UltraCDC are planned, and will be added as sibling subpackages.

The parent package's [github.com/chmduquesne/rollinghash/v4.Chunker] and
[github.com/chmduquesne/rollinghash/v4.ChunkWriter] provide the classic
single-mask, rolling-window CDC that works with any hash implementing
BatchBoundaries. The subpackages here cover algorithms that need their own
boundary primitive or their own hash.
*/
package cdc
