Documentation: [http://godoc.org/gopkg.in/chmduquesne/rollinghash.v1](http://godoc.org/gopkg.in/chmduquesne/rollinghash.v1)

Simply install with

    go get github.com/chmduquesne/rollinghash

Benchmark the rolling implementation versus the official vanilla (windows of 1024 bytes and 128 bytes)

    go test -bench . github.com/chmduquesne/rollinghash/adler32
    BenchmarkVanillaKB-4             3000000               584 ns/op        1752.43 MB/s
    BenchmarkRollingKB-4            100000000               17.5 ns/op      58621.67 MB/s
    BenchmarkVanilla128B-4          20000000                86.1 ns/op      11894.11 MB/s
    BenchmarkRolling128B-4          100000000               14.8 ns/op      69343.94 MB/s
    PASS
    ok      github.com/chmduquesne/rollinghash/adler32      7.422s
