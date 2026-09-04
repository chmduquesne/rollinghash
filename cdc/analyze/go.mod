// Nested module: the CDC comparison tool. Pulls github.com/PlakarKorp/go-cdc-chunkers
// (and its blake3 / cpuid deps) so the real upstream algorithms sit in the same
// table as this repo's. Kept separate so the main rollinghash module stays
// dependency-free. Run with:
//
//	cd cdc/analyze && go run .
module github.com/chmduquesne/rollinghash/v4/cdc/analyze

go 1.24.0

toolchain go1.24.4

require (
	github.com/PlakarKorp/go-cdc-chunkers v1.1.0
	github.com/chmduquesne/rollinghash/v4 v4.3.1
)

require (
	github.com/klauspost/cpuid/v2 v2.0.12 // indirect
	github.com/zeebo/blake3 v0.2.4 // indirect
)

replace github.com/chmduquesne/rollinghash/v4 => ../..
