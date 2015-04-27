Documentation: [http://godoc.org/github.com/chmduquesne/rollinghash](http://godoc.org/github.com/chmduquesne/rollinghash)

Simply install with

    go get github.com/chmduquesne/rollinghash

Benchmark the rolling implementation versus the official vanilla (windows of 1024 bytes and 128 bytes)

    go test -bench . github.com/chmduquesne/rollinghash/adler32
    PASS
    BenchmarkVanillaKB        200000              9059 ns/op         113.03 MB/s
    BenchmarkRollingKB      50000000                58.4 ns/op      17525.55 MB/s
    BenchmarkVanilla128B     2000000               956 ns/op        1070.93 MB/s
    BenchmarkRolling128B    50000000                58.0 ns/op      17664.38 MB/s
    ok      github.com/chmduquesne/rollinghash/adler32      10.786s
