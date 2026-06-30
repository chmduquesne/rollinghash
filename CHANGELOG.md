# Changelog

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
