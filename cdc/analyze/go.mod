// Nested module: the CDC comparison tool for this repo's own chunkers. Kept
// separate so the main rollinghash module stays dependency-free even as this
// tool's own dependencies change. Run with:
//
//	cd cdc/analyze && go run .
module github.com/chmduquesne/rollinghash/v4/cdc/analyze

go 1.24.0

toolchain go1.24.4

require github.com/chmduquesne/rollinghash/v4 v4.3.1

replace github.com/chmduquesne/rollinghash/v4 => ../..
