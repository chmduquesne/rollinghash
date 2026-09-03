// Nested module: pulls github.com/buildbarn/go-cdc (which needs the Go 1.26
// toolchain — `go` downloads it automatically) and its module-graph deps for
// head-to-head parity tests and benchmarks only. Kept separate so the main
// rollinghash module stays dependency-free. Run with:
//
//	cd cdc/compat/buildbarn/bench && go test ./... && go test -bench . ./...
module github.com/chmduquesne/rollinghash/v4/cdc/compat/buildbarn/bench

go 1.26.5

require (
	github.com/buildbarn/go-cdc v0.0.11-0.20260902072525-ff2e5ec8d45f
	github.com/chmduquesne/rollinghash/v4 v4.3.3
)

replace github.com/chmduquesne/rollinghash/v4 => ../../../..
