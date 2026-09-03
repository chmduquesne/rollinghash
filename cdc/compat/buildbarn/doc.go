// Package buildbarn is a drop-in replacement for the consumer API of
// github.com/buildbarn/go-cdc.
//
// Its ContentDefinedChunker, ChunkReader and Peeker interfaces, the
// New{Fast,Max,RepMax,RepMaxSfx,AsymmetricExtremum}ContentDefinedChunker
// constructors (and their NewSimple* variants), and the GearTable /
// SubstitutionBox types with FastContentDefinedChunkerGearTable,
// NoSubstitutionBox, NewSeededGearTable and NewSeededSubstitutionBox match that
// package's signatures, so migrating is:
//
//	import cdc "github.com/chmduquesne/rollinghash/v4/cdc/compat/buildbarn"
//
// and nothing else. Drive it the same way:
//
//	r := cdc.NewMaxContentDefinedChunker(&cdc.FastContentDefinedChunkerGearTable, 4<<10, 16<<10).
//		NewChunkReader(bufio.NewReaderSize(src, 64<<10))
//	for {
//		chunk, err := r.ReadNextChunk()
//		if err == io.EOF {
//			break
//		}
//		if err != nil {
//			return err
//		}
//		use(chunk)
//	}
//
// # How it works
//
// Each chunker wraps the corresponding typed package in this repo — cdc/fastcdc,
// cdc/maxcdc, cdc/repmaxcdc, cdc/repmaxsfxcdc, cdc/aecdc — which are byte-for-byte
// ports of buildbarn's Simple* reference chunkers. NewChunkReader adapts the
// Peeker to an io.Reader (a *bufio.Reader already is one) and feeds it to that
// package's streaming engine; ReadNextChunk is one step of its Next/Bytes loop.
// The returned slice aliases an internal buffer and is valid only until the next
// ReadNextChunk, exactly as in buildbarn (whose bytes come from Peek).
//
// FastCDC needs cdc/fastcdc's WithInclusiveBoundary option: buildbarn keeps the
// byte that clears the mask in the current chunk, whereas cdc/fastcdc defaults to
// the opposite (PlakarKorp) convention.
//
// # Differences from github.com/buildbarn/go-cdc
//
//   - Simple* and non-Simple* constructors return the same implementation.
//     buildbarn's optimized chunkers are performance rewrites of its Simple*
//     references and produce identical chunks; this package only ports the
//     references.
//   - DiscardUpToGuaranteedChunk (parallel-chunking synchronization, which
//     buildbarn supports for RepMaxCDC with a large enough horizon) is not
//     implemented. SupportsDiscardUpToGuaranteedChunk returns false for every
//     chunker and DiscardUpToGuaranteedChunk panics, matching buildbarn's
//     default for its other algorithms.
//   - NewFastContentDefinedChunker panics for a normalSizeBytes outside
//     {512, 1024, 2048, 4096, 8192, 16384, 32768, 65536}. buildbarn would fall
//     back to a zero mask (a cut at every byte).
//   - Parameter validation happens in NewChunkReader (when the underlying
//     chunker is built), not in the constructor.
//
// Callers who want the idiomatic, typed API should use cdc/fastcdc, cdc/maxcdc,
// cdc/repmaxcdc, cdc/repmaxsfxcdc and cdc/aecdc directly.
package buildbarn
