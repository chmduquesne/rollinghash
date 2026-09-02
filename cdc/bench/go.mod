// Nested module: pulls github.com/PlakarKorp/go-cdc-chunkers (and its blake3 /
// cpuid deps) for head-to-head parity tests and benchmarks only. Kept separate
// so the main rollinghash module stays dependency-free. Run with:
//
//	cd cdc/bench && go test ./... && go test -bench . ./...
module github.com/chmduquesne/rollinghash/v4/cdc/bench

go 1.23.0

toolchain go1.24.4

require (
	github.com/PlakarKorp/go-cdc-chunkers v1.1.0
	github.com/chmduquesne/rollinghash/v4 v4.3.1
	github.com/zeebo/blake3 v0.2.4
)

require github.com/klauspost/cpuid/v2 v2.0.12 // indirect

replace github.com/chmduquesne/rollinghash/v4 => ../..
