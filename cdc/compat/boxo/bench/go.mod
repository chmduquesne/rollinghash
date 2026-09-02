// Nested module: pulls github.com/ipfs/boxo for head-to-head parity tests and
// benchmarks only. Kept separate so the main rollinghash module stays
// dependency-free. Run with:
//
//	cd cdc/compat/boxo/bench && go test ./... && go test -bench . ./...
module github.com/chmduquesne/rollinghash/v4/cdc/compat/boxo/bench

go 1.24.6

require (
	github.com/chmduquesne/rollinghash/v4 v4.3.3
	github.com/ipfs/boxo v0.36.0
)

require (
	github.com/ipfs/go-log/v2 v2.9.1 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/whyrusleeping/chunker v0.0.0-20181014151217-fe64bd25879f // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	golang.org/x/sys v0.40.0 // indirect
)

replace github.com/chmduquesne/rollinghash/v4 => ../../../..
