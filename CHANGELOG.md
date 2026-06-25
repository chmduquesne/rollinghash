# Changelog

## v4.2.0 - 2026-06-25

### Added

- `Scanner`: batched bulk-hashing interface for rsync-style block search,
  with ~2× throughput vs `Roll` via ILP exploitation.
- `Chunker`: Content Defined Chunking interface with a fused boundary fast
  path for the same batched performance.
- `gearhash64`: new rolling hash.
- Fuzz tests covering all hashes and all interfaces.

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
