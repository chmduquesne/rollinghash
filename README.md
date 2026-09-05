[![CI](https://github.com/chmduquesne/rollinghash/actions/workflows/ci.yml/badge.svg)](https://github.com/chmduquesne/rollinghash/actions/workflows/ci.yml)
[![Coverage Status](https://codecov.io/gh/chmduquesne/rollinghash/branch/master/graph/badge.svg)](https://codecov.io/gh/chmduquesne/rollinghash)
[![GoDoc Reference](https://pkg.go.dev/badge/github.com/chmduquesne/rollinghash/v4.svg)](https://pkg.go.dev/github.com/chmduquesne/rollinghash/v4)
![Go 1.24+](https://img.shields.io/badge/go-1.24%2B-blue.svg)

# Rolling Hashes

## Philosophy

This package provides several rolling hashes. The API design philosophy is
to provide interfaces that are correct, fast and idiomatic. The hashes
are drop-in replacements whenever a builtin counterpart exists.

## Usage

### Roller

[`rollinghash.Hash`](https://godoc.org/github.com/chmduquesne/rollinghash/v4#Hash)
is the simplest interface: call `Roll` once per incoming byte and read the
updated hash immediately. It is ideal when the data is already in memory or when
throughput is not the bottleneck.

```golang
data := []byte("here is some data to roll on")
h := buzhash64.New()
n := 16 // window size

h.Write(data[:n])

for _, c := range data[n:] {
    h.Roll(c)
    fmt.Println(h.Sum64())
}
```

The hash maintains an internal copy of the rolling window. Use `WriteWindow` to
read it back out.

Beyond `Roll`, this library builds two higher-level capabilities on top of a
rolling hash: [Content-Defined Chunking](#content-defined-chunking) and
[Block Searching](#block-searching), each available as a pull interface
(owns an `io.Reader`, driven via `Next`) or a push interface (fed via
`Write`/`Close`, an `io.WriteCloser`) for callers who don't control how data
arrives, e.g. through an `io.Writer` you have to implement, a callback API,
or a network read loop.

A push interface's `Write` coalesces incoming bytes until a full batch is
available (or `Close` is called), so the batched rolling-hash computation
still runs on reasonably large batches even when the caller writes in small
pieces; it runs in O(n) regardless of `Write` call size or count. `Write`
returns `ErrClosed` if called after `Close`.

## Content-Defined Chunking

Content-Defined Chunking (CDC) splits a stream into variable-length chunks
at boundaries determined by local content rather than fixed offsets, so
inserting or deleting bytes only disturbs the chunks next to the edit,
which is the basis for deduplicating storage and efficient resync/transfer. This
library offers two ways to do it: a generic mask-based `Chunker`/
`ChunkWriter` built on top of any rolling hash, and a family of purpose-built
packages implementing specific published CDC algorithms.

### Rolling-hash CDC (Chunker / ChunkWriter)

#### Chunker

[`rollinghash.Chunker`](https://godoc.org/github.com/chmduquesne/rollinghash/v4#Chunker)
pulls from an `io.Reader`. It uses the same batch optimization as
`BatchRoller` (see [Block Searching](#block-searching)). The stream is split
wherever the rolling checksum matches a mask. Use `WithBoundaries` to keep
chunk sizes within a desired range.

```golang
// Generate 4KiB of pseudo-random data
data := make([]byte, 4096)
x := uint32(1)
for i := range data {
    x ^= x << 13; x ^= x >> 17; x ^= x << 5
    data[i] = byte(x)
}

// Cut where the low 8 bits of the rolling checksum are zero,
// keeping each chunk between 64 and 1024 bytes.
c := rollinghash.NewChunker(bytes.NewReader(data), buzhash64.New(), 56, 0xff,
    rollinghash.WithBoundaries(64, 1024))

for c.Next() {
    chunk := c.Bytes()
    if c.ContentDefined() {
        fmt.Printf("boundary at %d: sum=0x%x\n", c.Offset()+len(chunk), c.Sum())
    } else {
        fmt.Printf("max cut at %d\n", c.Offset()+len(chunk))
    }
}
if err := c.Err(); err != nil {
    log.Fatal(err)
}
```

Use `Reset` to reuse the chunker across multiple streams without extra
allocations.

#### ChunkWriter

[`rollinghash.ChunkWriter`](https://godoc.org/github.com/chmduquesne/rollinghash/v4#ChunkWriter)
does the same Content Defined Chunking as `Chunker`, over data delivered via
`Write`.

```golang
// Generate 4KiB of pseudo-random data
data := make([]byte, 4096)
x := uint32(1)
for i := range data {
    x ^= x << 13; x ^= x >> 17; x ^= x << 5
    data[i] = byte(x)
}

cw := rollinghash.NewChunkWriter(buzhash64.New(), 56, 0xff, rollinghash.WithBoundaries(64, 1024))

// Data can arrive in arbitrarily sized pieces; here it's split into two
// Writes to show that a chunk boundary straddling them is still found.
for _, piece := range [][]byte{data[:2000], data[2000:]} {
    cw.Write(piece)
    for cw.Next() {
        chunk := cw.Bytes()
        if cw.ContentDefined() {
            fmt.Printf("boundary at %d: sum=0x%x\n", cw.Offset()+len(chunk), cw.Sum())
        } else {
            fmt.Printf("max cut at %d\n", cw.Offset()+len(chunk))
        }
    }
}
cw.Close()
for cw.Next() {
    // final chunk(s), flushed now that the stream is known to be over
}
if err := cw.Err(); err != nil {
    log.Fatal(err)
}
```

Use `WithBatchSize` to control the coalescing threshold.

### Other CDC algorithms

Beyond `rollinghash.Chunker`/`ChunkWriter`, the `cdc/` subtree provides
purpose-built chunkers implementing several published CDC algorithms.
Each exposes the same `New` (pull, over an `io.Reader`) /
`NewChunkWriter` (push, via `Write`/`Close`) pair as the core package: the
`*Chunker` returned by `New` satisfies `rollinghash.Chunker`, and the
`*ChunkWriter` returned by `NewChunkWriter` satisfies
`rollinghash.ChunkWriter`, so code written against those two interfaces
works unchanged against any of them.

* [`cdc/fastcdc`](cdc/fastcdc) implements FastCDC (Xia et al., 2016), a
  windowless Gear fingerprint with normalized chunking and cut-point skipping.
* [`cdc/ultracdc`](cdc/ultracdc) implements UltraCDC, a hashless algorithm
  that slides an 8-byte window and cuts on Hamming distance to a constant
  pattern.
* [`cdc/jumpchunker`](cdc/jumpchunker) implements Jump Chunking (JC), a
  dual-mask Gear fingerprint that jumps ahead on a mask miss instead of
  stepping byte by byte, trading boundary placement for throughput.
* [`cdc/maxcdc`](cdc/maxcdc) implements MaxCDC, which cuts at the maximum of
  a windowless Gear fingerprint over a read-ahead horizon, giving a strict
  `[minSize, maxSize]` bound with no probabilistic tail.
* [`cdc/repmaxcdc`](cdc/repmaxcdc) / [`cdc/repmaxsfxcdc`](cdc/repmaxsfxcdc)
  implement RepMaxCDC, which refines MaxCDC into a strict `[minSize,
  2*minSize)` bound.
* [`cdc/aecdc`](cdc/aecdc) implements AE (Asymmetric Extremum), a hashless
  algorithm that cuts a fixed distance after the running maximum byte value
  settles.
* [`cdc/ramcdc`](cdc/ramcdc) implements RAM (Rapid Asymmetric Maximum), a
  hashless algorithm that pairs a fixed window maximum with an unbounded
  forward scan.
* [`cdc/maxpcdc`](cdc/maxpcdc) implements MAXP, a hashless algorithm that
  cuts at a local maximum byte over a sliding two-window range.

For example, `cdc/fastcdc` takes the same pull/push shape as the core
package, plus a `minSize`/`normalSize`/`maxSize` triple in place of a single
mask:

```golang
// Repeatable pseudo-random data (xorshift), so boundaries are stable.
data := make([]byte, 64*1024)
x := uint32(1)
for i := range data {
    x ^= x << 13
    x ^= x >> 17
    x ^= x << 5
    data[i] = byte(x)
}

c := fastcdc.New(bytes.NewReader(data), gearhash64.New(), 1024, 4096, 16384)

total, chunks := 0, 0
for c.Next() {
    total += len(c.Bytes())
    chunks++
}
if err := c.Err(); err != nil {
    log.Fatal(err)
}
fmt.Printf("split %d bytes into %d chunks\n", total, chunks)
// Output:
// split 65536 bytes into 14 chunks
```

`ramcdc`, `maxpcdc` and `aecdc` are accelerated by VectorCDC (AVX2 on amd64,
with a portable fallback elsewhere) without changing boundaries.

`cdc/analyze` is a standalone tool (separate Go module) that benchmarks
dedup ratio, chunk size distribution and resync behavior of every chunker
over a downloaded multi-format corpus.

See [CHANGELOG.md](CHANGELOG.md) and each package's doc comment for further
detail.

### Compatibility layers

`cdc/compat/` holds drop-in layers verified byte-for-byte against real
upstream implementations: [`buildbarn`](cdc/compat/buildbarn),
[`plakar`](cdc/compat/plakar), [`restic`](cdc/compat/restic) and
[`boxo`](cdc/compat/boxo). Each
mirrors its upstream package's own consumer API (same type and function
names, same signatures), so adopting one is just a change of import path.
For example, `cdc/compat/restic` mirrors `github.com/restic/chunker`:

```golang
import restic "github.com/chmduquesne/rollinghash/v4/cdc/compat/restic"

data := make([]byte, 48*1024)
x := uint32(1)
for i := range data {
    x ^= x << 13
    x ^= x >> 17
    x ^= x << 5
    data[i] = byte(x)
}

c := restic.New(bytes.NewReader(data), restic.Pol(0x3DA3358B4DC173),
    restic.WithBoundaries(512, 8192), restic.WithAverageBits(10))

total, chunks := 0, 0
for {
    chunk, err := c.Next(nil)
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatal(err)
    }
    total += int(chunk.Length)
    chunks++
}
fmt.Printf("split %d bytes into %d chunks\n", total, chunks)
// Output:
// split 49152 bytes into 36 chunks
```

## Block Searching

Block searching looks for a known block of bytes within a stream,
rsync-style: the rolling checksum acts as a cheap filter, and a secondary
check confirms the match.

### BatchRoller

[`rollinghash.BatchRoller`](https://godoc.org/github.com/chmduquesne/rollinghash/v4#BatchRoller)
pulls from an `io.Reader`. Computations are batched to exploit
instruction-level parallelism, achieving about twice the throughput of
`Roll`.

```golang
data := []byte("the quick brown fox jumps over the lazy dog")

needle := []byte("brown")
window := len(needle)

h := buzhash64.New()
h.Write(needle)
target := h.Sum64()

s := rollinghash.NewBatchRoller(bytes.NewReader(data), buzhash64.New(), window)
for s.Next() {
    sums, buf := s.Sums(), s.Bytes()
    for i, sum := range sums {
        if sum == target && bytes.Equal(buf[i:i+window], needle) {
            fmt.Printf("found %q at offset %d\n", needle, s.Offset()+i)
        }
    }
}
if err := s.Err(); err != nil {
    log.Fatal(err)
}
```

Within each batch, `Sums()[i]` is the checksum of `Bytes()[i:i+window]`, at
stream position `Offset()+i`. Use `WithBufferSize` to control the batch size and
`Reset` to reuse the batch roller across multiple streams without extra
allocations.

### BatchWriter

[`rollinghash.BatchWriter`](https://godoc.org/github.com/chmduquesne/rollinghash/v4#BatchWriter)
does the same rsync-style block search as `BatchRoller`, over data delivered
via `Write`.

```golang
needle := []byte("brown")
window := len(needle)

h := buzhash64.New()
h.Write(needle)
target := h.Sum64()

w := rollinghash.NewBatchWriter(buzhash64.New(), window)

// Data can arrive in arbitrarily sized pieces; boundary-straddling
// windows are still found across Write calls.
for _, p := range [][]byte{[]byte("the quick brown fox "), []byte("jumps over the lazy dog")} {
    w.Write(p)
    for w.Next() {
        sums, buf := w.Sums(), w.Bytes()
        for i, sum := range sums {
            if sum == target && bytes.Equal(buf[i:i+window], needle) {
                fmt.Printf("found %q at offset %d\n", needle, w.Offset()+i)
            }
        }
    }
}
w.Close()
for w.Next() {
    // drain any data buffered below one batch
}
if err := w.Err(); err != nil {
    log.Fatal(err)
}
```

Use `WithBufferSize` to control the coalescing threshold.

## Gotchas

### Call Write before the first Roll

The rolling window MUST be initialized by calling `Write` first (which
saves a copy). The byte leaving the rolling window is inferred from the
internal copy of the rolling window, which is updated with every call to
`Roll`.

### Use concrete types for maximum speed

Do NOT cast the result of `New()` to rollinghash.Hash. The Go compiler cannot
inline calls through an interface. This costs roughly 10% performance.

```golang
var h1 rollinghash.Hash
h1 = buzhash32.New()
h2 := buzhash32.New()

[...]

h1.Roll(b) // Not inlined (slow)
h2.Roll(b) // inlined (fast)
```

### Buzhash CDC: avoid window sizes that are multiples of the word size

When using `buzhash32` or `buzhash64` for Content Defined Chunking, do NOT
choose a window length that is a multiple of the word size (32 for
`buzhash32`, 64 for `buzhash64`).

Buzhash (cyclic polynomial) rolls its sum by rotating the word one bit per
byte, so the rotation wraps every word-size bytes. As a result, a run of
identical bytes at least as long as the window collapses the hash to a
single degenerate value (all-ones for odd multiples of the word size, zero
for even multiples), losing all entropy. Such runs are extremely common in
binary data (zero padding, `0xff` flash padding, alignment), so on typical
executables a 64-byte window makes `buzhash64` return
`0xffffffffffffffff` about 1% of the time, badly skewing the low bits.

This is inherent to the cyclic polynomial construction and cannot be fixed
by changing the byte table. Any window length that is not a multiple of
the word size avoids it (e.g. use 48 or 56 instead of 64).

### BatchRoller, Chunker, BatchWriter and ChunkWriter bypass the hash's rolling window

`BatchRoll` and `BatchBoundaries` are bulk operations that do not update
the hash's internal rolling window. After passing a hash to `NewBatchRoller`,
`NewChunker`, `NewBatchWriter` or `NewChunkWriter`, calling `h.WriteWindow()`
on that hash will not reflect the stream contents; its state is undefined.
Use `WindowSize()` on the roller/chunker/writer instead.

## Which hash to use

Benchmarked on 2026-08-31, linux/amd64, AMD Ryzen 7 PRO 7840U, CPU boost
disabled:

| Hash | Roll (MiB/s) | Chunker (MiB/s) | ChunkWriter (MiB/s) | BatchRoller (MiB/s) | BatchWriter (MiB/s) | Uniformly distributed | Parametrizable |
|---|---|---|---|---|---|---|---|
| `buzhash64` | 585 | 1017 | 1019 | 996 | 1019 | yes¹ | yes |
| `buzhash32` | 585 | 993 | 1003 | 995 | 999 | yes¹ | yes |
| `gearhash64` | 585 | 1018 | 1019 | 1022 | 1028 | yes | yes |
| `bozo32` | 587 | 788 | 794 | 894 | 910 | yes² | yes (single multiplier) |
| `bozo64` | 576 | 780 | 789 | 907 | 922 | yes² | yes (single multiplier) |
| `rabinkarp64` | 344 | 527 | 535 | 568 | 567 | yes | yes |
| `adler32` | 164 | 270 | 272 | 274 | 273 | **no**³ | no |

¹ Provided the window size is not a multiple of the word size (32 for `buzhash32`,
64 for `buzhash64`). See [Gotchas](#gotchas).

² For very small windows the output is bounded below 2⁶⁴ before modular wrapping
kicks in, so high bits are biased. For `bozo64` (multiplier `a ≈ 2³²`) wrapping
begins at window size 3; for `bozo32` (multiplier `a ≈ 2¹⁶`) at window size 5.
Any practical CDC window size is well above these thresholds.

³ `adler32` is not uniformly distributed for small windows: its two component sums
are bounded by `window × 255`, so the high bits of the output are always zero.
**Do not use `adler32` for CDC.** It is only useful for rsync-style block matching
where the peer already uses adler32 (e.g. the rsync protocol itself).

**`buzhash64`** and **`gearhash64`** are essentially tied for fastest, and
both solid defaults for CDC and block search.

**`gearhash64`** is also the popular choice from the CDC literature (see the
FastCDC paper); unlike buzhash it has no window-size gotcha (see
[Gotchas](#gotchas)) and is uniformly distributed.

**`bozo32`/`bozo64`** are very fast and parametrizable via a single integer
multiplier (`NewFromInt`), which is simpler than buzhash's 256-entry table but
sufficient to produce independent hash functions.

**`rabinkarp64`** is the slowest but lets you pick a specific irreducible
polynomial, which matters when you need to match an existing implementation
(e.g. restic).

## License

This code is delivered to you under the terms of the MIT public license,
except the `rabinkarp64` subpackage, which has been adapted from
[restic](https://github.com/restic/chunker) (BSD 2-clause "Simplified").

## Notable users

This library is used by a wide variety of tools, for production and
scientific purposes.

* [syncthing](https://syncthing.net/), a decentralized synchronisation
  solution
* [muscato](https://github.com/kshedden/muscato), a genome analysis tool
* [kopia](https://github.com/kopia/kopia), a backup tool
* [pachyderm](https://github.com/pachyderm/pachyderm), a data science
  platform

If you are using successfully, let me know and I will happily put a link
here!
