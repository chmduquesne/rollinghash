[![CI](https://github.com/chmduquesne/rollinghash/actions/workflows/ci.yml/badge.svg)](https://github.com/chmduquesne/rollinghash/actions/workflows/ci.yml)
[![Coverage Status](https://codecov.io/gh/chmduquesne/rollinghash/branch/master/graph/badge.svg)](https://codecov.io/gh/chmduquesne/rollinghash)
[![GoDoc Reference](https://pkg.go.dev/badge/github.com/chmduquesne/rollinghash/v4.svg)](https://pkg.go.dev/github.com/chmduquesne/rollinghash/v4)
![Go 1.21+](https://img.shields.io/badge/go-1.21%2B-blue.svg)

# Rolling Hashes

## Philosophy

This package contains several various rolling hashes. The API design philosophy
is to stick as closely as possible to the interface provided by the builtin hash
package (the hashes implemented here are effectively drop-in replacements for
their builtin counterparts), while providing simultaneously the highest speed
and simplicity.

## Usage

### Roll

[`rollinghash.Hash`](https://godoc.org/github.com/chmduquesne/rollinghash/v4#Hash)
is the simplest interface: call `Roll` once per incoming byte and read the
updated hash immediately. It is the right choice when the data is already in
memory or when throughput is not the bottleneck. For stream processing at the
highest speed, prefer the Scanner or Chunker interfaces below.

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

### Scanner

The
[`rollinghash.Scanner`](https://godoc.org/github.com/chmduquesne/rollinghash/v4#Scanner)
is designed for searching a block within a stream, rsync-style: the rolling
checksum acts as a cheap filter, and a secondary check (e.g. byte comparison)
confirms the match. It is shaped like a
[`bufio.Scanner`](https://golang.org/pkg/bufio/#Scanner) and batches
computations to exploit instruction-level parallelism. It is about twice as fast
as `Roll`.

```golang
data := []byte("the quick brown fox jumps over the lazy dog")

needle := []byte("brown")
window := len(needle)

h := bozo64.New()
h.Write(needle)
target := h.Sum64()

s := rollinghash.NewScanner(bytes.NewReader(data), bozo64.New(), window)
for s.Scan() {
    sums, buf := s.Sums(), s.Bytes()
    for i, sum := range sums {
        if sum == target && bytes.Equal(buf[i:i+window], needle) {
            fmt.Printf("found %q at offset %d\n", needle, i)
        }
    }
}
if err := s.Err(); err != nil {
    log.Fatal(err)
}
```

Within each batch, `Sums()[i]` is the checksum of `Bytes()[i:i+window]`.
Use `Buffer` to control the batch size and `Reset` to reuse the scanner
across multiple streams without extra allocations.

### Chunker

The
[`rollinghash.Chunker`](https://godoc.org/github.com/chmduquesne/rollinghash/v4#Chunker)
is designed for Content Defined Chunking (CDC). It also operates on a
stream and uses the same batch optimization as the Scanner. The stream is
split wherever the rolling checksum matches a mask, with chunk sizes kept
within `[min, max]`.

```golang
// Cut where the low 8 bits of the rolling checksum are zero,
// keeping each chunk between 64 and 1024 bytes.
c := rollinghash.NewChunker(bytes.NewReader(data), bozo64.New(), 32, 0xff, 64, 1024)

for c.Next() {
    chunk := c.Chunk()
    if c.AtMask() {
        fmt.Printf("boundary: sum=0x%x, len=%d\n", c.Sum(), len(chunk))
    } else {
        fmt.Printf("max cut: len=%d\n", len(chunk))
    }
}
if err := c.Err(); err != nil {
    log.Fatal(err)
}
```

Use `Reset` to reuse the chunker across multiple streams without extra
allocations.

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

## Which hash to use

| Hash | Chunker (MB/s) | Scanner (MB/s) | Uniformly distributed | Parametrizable |
|---|---|---|---|---|
| `buzhash64` | 1465 | 1424 | yes¹ | yes |
| `buzhash32` | 1444 | 1398 | yes¹ | yes |
| `gearhash64` | 1210 | 1421 | yes | yes |
| `bozo64` | 1135 | 1275 | yes² | yes (single multiplier) |
| `bozo32` | 1141 | 1287 | yes² | yes (single multiplier) |
| `rabinkarp64` | 693 | 807 | yes | yes |
| `adler32` | 408 | 386 | **no**³ | no |

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

**`buzhash64`** is the fastest overall and a solid default for both CDC and
block search.

**`gearhash64`** is the popular choice from the CDC literature (see the FastCDC
paper). It is essentially as fast as buzhash on the Scanner, has no window-size
gotcha, and is uniformly distributed.

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

If you are using succesfully, let me know and I will happily put a link
here!
