# Changelog

## v4.4.0 - unreleased

### Added

- `cdc`: new umbrella tree of content-defined-chunking algorithms, each in its
  own subpackage. Every `*Chunker` implements `rollinghash.Chunker` and every
  `*ChunkWriter` (each package's `NewChunkWriter`, the push-based counterpart
  fed via `Write`/`Close`) implements `rollinghash.ChunkWriter`, so one loop
  works across all algorithms and both drive styles. Boundaries are
  byte-for-byte compatible with the matching algorithm in
  `PlakarKorp/go-cdc-chunkers` — verified against the real package by the
  `cdc/compat/plakar/bench` nested module (kept separate so its blake3/cpuid deps never enter
  the dependency-free main module). Head-to-head throughput on 1 MiB of random
  data vs go-cdc-chunkers v1.1.0, both allocation-free: JC ~9.85 GB/s vs ~5.0
  (1.95×), FastCDC ~3.95 vs ~2.6 (1.52×), UltraCDC ~1.07 vs ~1.10 (0.97×).
- `cdc/jumpchunker`: Jump Chunking (JC). Windowless accumulating Gear
  fingerprint with a dual-mask jump that skips regions provably free of
  boundaries. `jumpchunker.New` takes any hash exposing `Table()` (`gearhash64`
  does) and panics otherwise. `WithJumpMask` pins the boundary mask and jump
  stride for interop; `WithSpecFaithful` selects the paper's Algorithm 1
  (scan a sub-normalSize tail instead of emitting it whole).
- `cdc/fastcdc`: FastCDC (Xia et al. 2016). Normalized chunking — a stricter
  mask below the target size, a looser one above — over a windowless Gear
  fingerprint, with the first `minSize` bytes of every chunk skipped. Masks are
  derived from `normalSize` by default; `WithMasks` pins them, and
  `LegacyMaskS`/`LegacyMaskL` reproduce the reference 8 KiB tuning.
- `cdc/ultracdc`: UltraCDC. Slides an 8-byte window, tracks its Hamming
  distance to `0xAA…AA`, cuts when that distance clears a small mask or on a
  long run of identical windows. Uses no rolling hash. `WithSpecFaithful`
  rounds boundaries up to the 8-byte window.
- `cdc/maxcdc`: MaxCDC, from `buildbarn/go-cdc`. Over the same windowless
  accumulating Gear fingerprint, cuts before the position where the fingerprint
  is largest within `[minSize, maxSize]` from the chunk start — no boundary
  mask, so lengths land in `[minSize, maxSize]` with no probabilistic tail.
  Same hash/`Table()` contract as the other Gear chunkers; boundaries verified
  against a port of upstream's `NewSimpleMaxContentDefinedChunker`.
- `cdc/repmaxcdc`: RepMaxCDC ("repeated maximum"), the tight-bound chunker from
  `buildbarn/go-cdc` proposed for Bazel's remote-execution protocol. Cuts at the
  position where the windowless accumulating Gear fingerprint is maximal within
  a read-ahead horizon, repeating the search until every chunk lands in
  `[minSize, 2*minSize)`. Parameters are `minSize` and a horizon size (the
  horizon only controls quality and can be raised freely). `repmaxcdc.New` takes
  any hash exposing `Table()` and panics otherwise; boundaries match
  `buildbarn/go-cdc`'s `NewRepMaxContentDefinedChunker` for the same table,
  verified against a port of its `NewSimpleRepMaxContentDefinedChunker`
  reference (upstream requires Go 1.26, so it is not pulled as a nested module).
- `cdc/compat/plakar`: drop-in for `PlakarKorp/go-cdc-chunkers`' consumer API. Its
  `NewChunker`/`NewChunkerBuffer`, `*Chunker`, `ChunkerOpts`, `ErrMinSize` /
  `ErrMaxSize` / `ErrNormalSize`, and the `Next`/`Split`/`Copy` method
  signatures match that package's, so migrating is `import chunkers
  "…/cdc/compat/plakar"` and nothing else. Produces byte-identical chunks for the
  `fastcdc`, `ultracdc`, and `jc` families (including the `-v1.0.0` /
  `-v1.1.0` variants). Keyed FastCDC is not supported (those names return an
  error).
- `cdc/compat/restic`: drop-in for `github.com/restic/chunker`'s consumer API.
  `New`/`NewWithBoundaries`, `*Chunker`, `Chunk`, `Pol`,
  `RandomPolynomial`/`DerivePolynomial`, `MinSize`/`MaxSize`, the
  `WithBoundaries`/`WithAverageBits`/`WithBuffer` options and the
  `Next`/`Reset`/`SetAverageBits` methods match that package's, so migrating is
  `import chunker "…/cdc/compat/restic"` and nothing else. Produces
  byte-identical boundaries (Rabin fingerprint over a degree-53 polynomial,
  64-byte window) via `rabinkarp64` + `rollinghash.NewChunker`, verified against
  the real package by the `cdc/compat/restic/bench` nested module. `Chunk.Cut`
  differs only on a final chunk shorter than `MinSize`, where restic returns a
  value its own source calls meaningless. The `BaseChunker`/`NewBase` API is not
  mirrored.
- `rabinkarp64.Pol.MarshalJSON`/`UnmarshalJSON`: encode a polynomial as a quoted
  lowercase-hex string, matching restic/chunker, so a polynomial from a restic
  repository config round-trips through `rabinkarp64.Pol`.
- `cdc/compat/boxo`: drop-in for `github.com/ipfs/boxo/chunker` (package
  `chunk`). `Splitter`, `NewSizeSplitter`/`NewRabin`/`NewRabinMinMax`/`NewBuzhash`,
  `DefaultSplitter`, `SizeSplitterGen`, `Chan`, `FromString` and `Register` match
  that package's signatures, so migrating is `import chunk "…/cdc/compat/boxo"`
  and nothing else. Produces byte-identical chunks — and therefore identical
  IPFS CIDs — for all three built-in splitters: rabin (`rabinkarp64` over
  `IpfsRabinPoly`, 16-byte window), buzhash (`buzhash32` with boxo's table,
  32-byte window), and fixed-size. Verified against `github.com/ipfs/boxo`
  itself by the `cdc/compat/boxo/bench` nested module. Chunk slices handed back
  by `NextBytes` are always caller-owned copies; the accumulator buffer is
  pooled (like boxo's go-buffer-pool use). On 8 MiB of random data the rabin
  splitter runs ~3× faster than boxo's (`whyrusleeping/chunker`, unmaintained
  since 2018) and buzhash is at parity.
- `cdc/compat/duplicacy`: drop-in for the chunk boundaries of
  `github.com/gilbertchen/duplicacy`'s `ChunkMaker`. `CreateChunkMaker` with the
  `WithChunkSeed` / `WithAverageChunkSize` / `WithMinimumChunkSize` /
  `WithMaximumChunkSize` / `WithBuffer` options and the `AddData(reader,
  sendChunk)` / `Reset` methods mirror Duplicacy's, and it splits a stream at the
  same offsets for a given seed and size triple. Duplicacy's buzhash splitter
  (window = minimum chunk size, single mask `averageChunkSize-1`, SHA-256 table
  chain from the repository `ChunkSeed`) is reproduced with `buzhash64` +
  `rollinghash.NewChunkWriter`. `Chunk.Hash` is the buzhash of the window at the
  cut (the value Duplicacy tests against its `hashMask`); the chunk *content*
  hash, ID and file name are out of scope. Verified against an independent
  reimplementation of the algorithm (`refChunks` in the tests) rather than a
  nested bench module: Duplicacy ships no consumable Go module (its pre-`go.mod`
  tags no longer build against current transitive deps, its `v3` tags lack a
  `/v3` path). That same reference is the throughput baseline —
  `BenchmarkChunkMaker` runs `compat` and `reference` side by side. The buzhash
  window is the whole minimum chunk size (megabytes), so `CreateChunkMaker`
  defaults the hashing batch to ~8 windows to amortise `BatchBoundaries`
  priming; `compat` still runs at ~0.85x of the single-pass `reference` (the
  per-batch re-prime and the per-cut `Chunk.Hash` recompute), and the
  multiple-of-64 window precludes SIMD, so it is slower in absolute terms than
  the small-window CDC algorithms here.
- `cdc/{fastcdc,ultracdc,jumpchunker}.NewChunkWriter(…)`: push-based
  `rollinghash.ChunkWriter` counterparts to `New`, fed via `Write`/`Close`
  instead of an `io.Reader` (for callers whose data arrives in pieces, e.g.
  content-addressable storage writers). Same parameters, same boundaries; share
  the streaming engine with the pull-based `Chunker`.
- `cdc/{fastcdc,ultracdc,jumpchunker}.WithBuffer([]byte)`: supply the working
  buffer, adopted when its capacity is large enough, for reuse across streams.
- `WithBuffer([]byte)`: functional option for `NewChunker` / `NewChunkWriter`
  supplying the byte accumulator, so callers that create many short-lived
  chunkers can share one allocation (e.g. a `sync.Pool`) instead of each growing
  a buffer from nothing. It does not change the chunk stream; a chunker reused
  via `Reset` already keeps its buffer and needs nothing.
- `gearhash64.Table()`: returns a copy of the hash's 256-entry Gear table, the
  inverse of `NewFromUint64Array`.

### Changed

- `cdc/{jumpchunker,fastcdc,ultracdc}` (unreleased): `Sum()` now matches the
  parent `Chunker`/`ChunkWriter` semantics from v4.3.2: it returns the
  algorithm's rolling value for the window ending at the chunk's cut regardless
  of how the cut was chosen. At a content-defined boundary it is still the
  mask-tested value; at a forced cut at `max` or the final chunk it is
  recomputed over the last `WindowSize` bytes (Gear fingerprint for JC/FastCDC,
  Hamming distance to `0xAA` for UltraCDC) instead of being 0, letting a caller
  confirm the window did not satisfy the mask. It is 0 only for a final chunk
  lying within `WindowSize` bytes of the start of the stream. The shared engine
  now retains `WindowSize-1` bytes of lead-in across buffer compaction so this
  window is available even when it straddles the previous chunk's end. No change
  to chunk boundaries.
- `cdc/{jumpchunker,fastcdc,ultracdc}` (unreleased): confirmed to match the
  parent `Chunker`/`ChunkWriter` v4.3.3 semantics — a stream shorter than
  `WindowSize` yields its bytes as one final, non-content-defined chunk
  (`Sum()` 0), and only a truly empty stream yields no chunks. The shared
  streaming engine (`cdc/internal/chunkcore`) already emitted trailing bytes
  unconditionally, so no code change was needed; the edge-case tests for all
  three algorithms and both drive styles now assert it explicitly.
- `cdc/jumpchunker` (unreleased): rewritten on the new shared `cdc` streaming
  engine and realigned to plakar's cut semantics — the boundary byte is now the
  first byte of the next chunk, not the last byte of the current one, and a
  sub-normalSize final segment is emitted whole by default. `New` now takes a
  hash exposing `Table()` instead of `JumpBoundaries`.
- `cdc` Gear scan: the FastCDC and JC inner loops now share
  `cdc/internal/gearscan.Scan4`, a 4-byte-unrolled scan that issues the
  dependent `data[]`/Gear-table loads as a group so they pipeline instead of
  running one per branch. On 1 MiB of random data, allocation-free: JC
  ~5.7 GB/s (old batch-incremental design) → ~9.8; FastCDC ~2.9 → ~4.0.
- `cdc` streaming engine: the shared buffer keeps more compaction slack
  (`2*MaxSize + 256 KiB`, was `+32 KiB`), so the consumed prefix is moved less
  often and moves a smaller fraction of what it frees each time. No change to
  chunk boundaries. `BenchmarkChunker` on 1 MiB of random data (boost off):
  JC ~6.5 → ~6.8 GB/s (+4.8%), FastCDC +~2%; UltraCDC unchanged (compute-bound).
  Costs ~224 KiB more resident buffer per chunker.

### Removed

- `gearhash64.JumpBoundaries` (unreleased): its JC-specific logic
  (maskC/maskJ/jumpLen/minStep) does not belong in a hash package. The JC scan
  now lives in `cdc/jumpchunker`, reading the table via `Table()`.

## v4.3.3 - 2026-09-02

### Fixed

- `Chunker`/`ChunkWriter`: a stream shorter than `window` now yields its bytes
  as a single final chunk instead of no chunk at all. A chunk is just a byte
  range, so it exists whether or not a rolling checksum can be computed over
  its end; the old behavior silently dropped the entire content of a tiny
  stream. `Sum()` is still 0 for such a chunk and `ContentDefined()` is false.
  This matches the `Chunker`'s own reference implementation and the documented
  "the trailing bytes of the stream form a final chunk". A truly empty stream
  still yields no chunks.

## v4.3.2 - 2026-09-02

### Fixed

- `Chunker`/`ChunkWriter`: `Sum()` now always returns the true rolling digest
  of the window ending at the chunk's cut. Two cases previously returned 0:
  a content-defined boundary whose window straddled the previous chunk's end
  (possible when `min` < `window`), and any forced cut at `max` or final
  chunk. It now returns 0 only for a final chunk whose stream has fewer than
  `window` bytes. This lets callers read the checksum at a forced cut to
  confirm it did not satisfy the mask. No change to the chunk boundaries
  produced.

## v4.3.1 - 2026-08-31

### Changed

- `Chunker`/`ChunkWriter`: `WithBoundaries`' `min` is no longer a pure
  post-filter. The first `min-window` bytes of every chunk can't hold an
  acceptable boundary, so they are skipped instead of being fed to
  `BatchBoundaries`; a large `min` now speeds chunking roughly in proportion
  to the fraction of the stream it covers (e.g. ~1.3–1.5× at `min` = 64 KiB
  with an 8 KiB average spacing between mask hits, measured by
  `BenchmarkChunker`'s new `/bigmin` variant). Boundary detection is now
  lazy, driven by `Next`, replacing the previous carry/scratch
  double-buffering in `feed`. No change to the chunk boundaries produced.

## v4.3.0 - 2026-08-31

### Added

- `ChunkWriter`: push-based counterpart to `Chunker`, satisfied by
  `NewChunkWriter`. Fed via `Write`/`Close` (`io.WriteCloser`) instead of
  owning an `io.Reader`, for callers whose data arrives in caller-controlled
  pieces (e.g. content-addressable storage writers). Shares its
  boundary-finding core with `Chunker`. `Write` coalesces bytes until
  `WithBatchSize` (default 16 KiB, shared with `NewChunker`'s read-buffer
  size) is reached, or `Close` is called, so `BatchBoundaries` is invoked on
  reasonably large batches even when the caller writes in small pieces;
  runs in O(n) regardless of Write call size or count, feeding directly from
  the caller's slice rather than copying it, and keeps its internal
  coalescing buffer bounded to roughly one batch regardless of how large
  individual Write calls are. `Write` returns `ErrClosed` if called after
  `Close`.
- `BatchWriter`: push-based counterpart to `BatchRoller`, satisfied by
  `NewBatchWriter`. Fed via `Write`/`Close` instead of owning an
  `io.Reader`. Shares its batching core with `BatchRoller`. `Write`
  coalesces the same way as `ChunkWriter`, reusing `WithBufferSize` (default
  64 KiB) as the coalescing threshold; `Write` returns `ErrClosed` if called
  after `Close`.
- `WithBatchSize`: functional option (shared by `NewChunker` and
  `NewChunkWriter`) setting the read-buffer size (`NewChunker`) or
  write-coalescing threshold (`NewChunkWriter`). Default 16 KiB. Same role
  as `WithBufferSize` for `NewBatchRoller`/`NewBatchWriter`; the two
  options can't share one name (distinct option types, no function
  overloading in Go), so both are documented as equivalents of each other.
  For `ChunkWriter`/`BatchWriter`, the low end of this range is steep: the
  technical minimum (`window`) gives up most of the throughput a batch size
  of just a few KiB would already recover; see the doc comments.
- `ErrClosed`: sentinel error returned by `ChunkWriter.Write`/
  `BatchWriter.Write` after `Close`.

### Changed

- `gearhash64.Roll`: throughput improvement via a precomputed
  shifted-leaving-byte table, computed once per `Write` instead of
  shifting by a variable count on every `Roll`.
- `chunker.go`/`batchroller.go`: internal refactor extracting the
  source-agnostic boundary-finding/batching logic into `chunkerCore`/
  `batchRollerCore`, now shared with `ChunkWriter`/`BatchWriter`. No
  behavior change for `Chunker`/`BatchRoller`.

## v4.2.0 - 2026-06-30

### Added

- `BatchRoller`: interface for batch rolling-hash iteration, satisfied by
  `NewBatchRoller`. Exposes `Next`, `Bytes`, `Sums`, `Offset`, `WindowSize`, `Err`, and `Reset`.
- `NewBatchRoller`: batch-hashing implementation for rsync-style block
  search, with ~2× throughput vs `Roll` via ILP exploitation. Requires the
  hash to implement `BatchRoll`; panics at construction otherwise. Returns
  the `BatchRoller` interface. Accepts variadic options.
- `WithBufferSize`: functional option to control the
  internal batch buffer size in bytes (default 64 KiB).
- `Chunker`: interface for Content Defined Chunking, satisfied by
  `NewChunker`. Exposes `Next`, `Bytes`, `ContentDefined`, `Sum`, `Offset`,
  `WindowSize`, `Err`, and `Reset`. Intended to be the common type for CDC implementations;
  future algorithms (e.g. Jump Chunking) will implement it too.
- `NewChunker`: CDC implementation with a fused boundary fast path,
  achieving ~2× throughput vs a naive rolling-hash scan via batched
  `BatchBoundaries`. Requires the hash to implement `BatchBoundaries`;
  panics at construction otherwise. Returns the `Chunker` interface.
  Accepts variadic options.
- `WithBoundaries`: functional option to set the minimum
  and maximum chunk size (defaults: 0 and `math.MaxInt`).
- `gearhash64`: new rolling hash.
- Fuzz tests covering all hashes and all interfaces.
- `bozo32.NewFromInt`, `bozo64.NewFromInt`: godoc now documents the odd->1
  constraint and the reason (even multipliers accumulate factors of 2 in
  window powers; a=1 collapses the hash to a bounded sum).
- `FuzzNewFromInt`, `FuzzNewFromIntCDC`: fuzz tests for `bozo32` and
  `bozo64` verifying correctness of `Roll`/`BatchRoll` for arbitrary
  multipliers and geometric trailing-zero decay for odd multipliers >1.

### Changed

- `buzhash32.Roll`, `buzhash64.Roll`: throughput improvement via a
  precomputed leaving-byte table.
- Default benchmark window size changed to 56 to avoid the buzhash
  word-size degeneracy (see Gotchas in README).

## v4.1.1

### Fixed

- The module now correctly follows Go's semantic import versioning. The
  import path is `github.com/chmduquesne/rollinghash/v4`. v4.1.0 shipped
  a `go.mod` with the unsuffixed path, making it uninstallable via
  `go get`.

## v4.1.0

### Added

- `bozo64`: new rolling hash, equally fast as `bozo32` but with 64-bit output.
- Vulnerability checking via [govulncheck-action](https://github.com/golang/govulncheck-action).
- Dependency checking via [dependabot](https://github.com/dependabot).

### Changed

- `rabinkarp64`: internals simplified (`rabinkarp64.Pol.Deg()`); +42% throughput.
- `adler32.Roll`: +5% throughput (algebraic simplifications).
- `buzhash32.Roll`, `buzhash64.Roll`: +24% throughput (`math/bits` rotation).
- Test suite extended for improved coverage.

## v4.0.0

### Changed

- `Write` is now fully consistent with `hash.Hash`: it appends data to the
  existing window instead of reinitializing it. Use `Reset` to clear the window.
- `Roll` on an empty window now panics instead of silently producing wrong results.
