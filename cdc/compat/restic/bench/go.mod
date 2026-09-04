// Nested module: pulls github.com/restic/chunker for head-to-head parity tests
// and benchmarks only. Kept separate so the main rollinghash module stays
// dependency-free. Run with:
//
//	cd cdc/compat/restic/bench && go test ./... && go test -bench . ./...
module github.com/chmduquesne/rollinghash/v4/cdc/compat/restic/bench

go 1.24.0

toolchain go1.24.4

require (
	github.com/chmduquesne/rollinghash/v4 v4.3.3
	github.com/restic/chunker v0.5.0
)

replace github.com/chmduquesne/rollinghash/v4 => ../../../..
