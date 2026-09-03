package buildbarn

import (
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/cdc/aecdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/fastcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/maxcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/repmaxcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/repmaxsfxcdc"
)

// ChunkReader reads a stream as a sequence of content-defined chunks. It mirrors
// github.com/buildbarn/go-cdc's ChunkReader.
type ChunkReader interface {
	// ReadNextChunk returns the next chunk, or io.EOF once the stream is
	// exhausted. The returned slice aliases an internal buffer and is only
	// valid until the next call.
	ReadNextChunk() ([]byte, error)
}

// Peeker is the buffered-reader view a ChunkReader consumes. It mirrors
// github.com/buildbarn/go-cdc's Peeker; *bufio.Reader satisfies it.
type Peeker interface {
	Discard(n int) (int, error)
	Peek(n int) ([]byte, error)
}

// ContentDefinedChunker is a configured chunking function. It mirrors
// github.com/buildbarn/go-cdc's ContentDefinedChunker.
type ContentDefinedChunker interface {
	NewChunkReader(peeker Peeker) ChunkReader
	SupportsDiscardUpToGuaranteedChunk() bool
	DiscardUpToGuaranteedChunk(peeker Peeker) error
	GetMaximumPeekSizeBytes() int
}

// nonSynchronizable is embedded by every chunker here: this package does not
// implement buildbarn's parallel-chunking synchronization (DiscardUpToGuaranteedChunk).
type nonSynchronizable struct{}

func (nonSynchronizable) SupportsDiscardUpToGuaranteedChunk() bool { return false }

func (nonSynchronizable) DiscardUpToGuaranteedChunk(Peeker) error {
	panic("This Content-Defined Chunking function does not support discarding up to a guaranteed chunk")
}

// contentDefinedChunker is the single implementation behind every constructor.
type contentDefinedChunker struct {
	nonSynchronizable
	peek int
	make func(io.Reader) rollinghash.Chunker
}

func (c *contentDefinedChunker) GetMaximumPeekSizeBytes() int { return c.peek }

func (c *contentDefinedChunker) NewChunkReader(peeker Peeker) ChunkReader {
	return &chunkReader{c: c.make(asReader(peeker))}
}

type chunkReader struct{ c rollinghash.Chunker }

func (r *chunkReader) ReadNextChunk() ([]byte, error) {
	if !r.c.Next() {
		if err := r.c.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	return r.c.Bytes(), nil
}

// asReader turns a Peeker into an io.Reader. The common case (*bufio.Reader) is
// already one; anything else gets a Peek+Discard shim.
func asReader(p Peeker) io.Reader {
	if r, ok := p.(io.Reader); ok {
		return r
	}
	return &peekerReader{p: p}
}

type peekerReader struct{ p Peeker }

func (r *peekerReader) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	// Block for at least one byte, surfacing EOF and read errors.
	if _, err := r.p.Peek(1); err != nil {
		return 0, err
	}
	d, _ := r.p.Peek(len(b)) // a short read (e.g. bufio.ErrBufferFull) is fine
	n := copy(b, d)
	if _, err := r.p.Discard(n); err != nil {
		return n, err
	}
	return n, nil
}

// NewFastContentDefinedChunker returns a FastCDC8KB chunker, mirroring
// github.com/buildbarn/go-cdc. Chunk sizes are [normalSizeBytes/4,
// normalSizeBytes*4] and normalSizeBytes must be one of 512, 1024, 2048, 4096,
// 8192, 16384, 32768, 65536 (buildbarn only defines its masks for those; New
// panics otherwise, where buildbarn would cut at every byte).
//
// Backed by cdc/fastcdc with fastcdc.WithInclusiveBoundary (buildbarn keeps the
// mask-clearing byte in the current chunk).
func NewFastContentDefinedChunker(gearTable *GearTable, normalSizeBytes int) ContentDefinedChunker {
	minSize, maxSize := normalSizeBytes/4, normalSizeBytes*4
	maskS, maskL, ok := fastCDCMasks(normalSizeBytes)
	if !ok {
		panic("buildbarn: NewFastContentDefinedChunker: normalSizeBytes must be one of 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536")
	}
	return &contentDefinedChunker{
		peek: maxSize,
		make: func(r io.Reader) rollinghash.Chunker {
			return fastcdc.New(r, gearTable.hash(), minSize, normalSizeBytes, maxSize,
				fastcdc.WithMasks(maskS, maskL), fastcdc.WithInclusiveBoundary())
		},
	}
}

// NewMaxContentDefinedChunker returns a MaxCDC chunker (cut before the maximal
// Gear fingerprint in [minSizeBytes, maxSizeBytes]), mirroring
// github.com/buildbarn/go-cdc. Backed by cdc/maxcdc.
func NewMaxContentDefinedChunker(gearTable *GearTable, minSizeBytes, maxSizeBytes int) ContentDefinedChunker {
	return &contentDefinedChunker{
		peek: minSizeBytes + maxSizeBytes,
		make: func(r io.Reader) rollinghash.Chunker {
			return maxcdc.New(r, gearTable.hash(), minSizeBytes, maxSizeBytes)
		},
	}
}

// NewSimpleMaxContentDefinedChunker is identical to NewMaxContentDefinedChunker
// (buildbarn's simple and optimized implementations produce the same chunks).
func NewSimpleMaxContentDefinedChunker(gearTable *GearTable, minSizeBytes, maxSizeBytes int) ContentDefinedChunker {
	return NewMaxContentDefinedChunker(gearTable, minSizeBytes, maxSizeBytes)
}

// NewRepMaxContentDefinedChunker returns a RepMaxCDC chunker (repeated Gear
// maximum, chunks strictly in [minSizeBytes, 2*minSizeBytes)), mirroring
// github.com/buildbarn/go-cdc. Backed by cdc/repmaxcdc.
func NewRepMaxContentDefinedChunker(gearTable *GearTable, minSizeBytes, horizonSizeBytes int) ContentDefinedChunker {
	return &contentDefinedChunker{
		peek: 2*minSizeBytes + horizonSizeBytes,
		make: func(r io.Reader) rollinghash.Chunker {
			return repmaxcdc.New(r, gearTable.hash(), minSizeBytes, horizonSizeBytes)
		},
	}
}

// NewSimpleRepMaxContentDefinedChunker is identical to
// NewRepMaxContentDefinedChunker.
func NewSimpleRepMaxContentDefinedChunker(gearTable *GearTable, minSizeBytes, horizonSizeBytes int) ContentDefinedChunker {
	return NewRepMaxContentDefinedChunker(gearTable, minSizeBytes, horizonSizeBytes)
}

// NewRepMaxSfxContentDefinedChunker returns a RepMaxSfxCDC chunker (repeated
// lexicographic-suffix maximum, chunks strictly in [minSizeBytes,
// 2*minSizeBytes)), mirroring github.com/buildbarn/go-cdc. Every compared byte
// is mapped through substitutionBox (nil or NoSubstitutionBox = identity).
// Backed by cdc/repmaxsfxcdc.
func NewRepMaxSfxContentDefinedChunker(substitutionBox *SubstitutionBox, minSizeBytes, horizonSizeBytes int) ContentDefinedChunker {
	var opts []repmaxsfxcdc.Option
	if substitutionBox != nil && *substitutionBox != NoSubstitutionBox {
		opts = append(opts, repmaxsfxcdc.WithSubstitutionBox([256]byte(*substitutionBox)))
	}
	return &contentDefinedChunker{
		peek: 2*minSizeBytes + horizonSizeBytes,
		make: func(r io.Reader) rollinghash.Chunker {
			return repmaxsfxcdc.New(r, minSizeBytes, horizonSizeBytes, opts...)
		},
	}
}

// NewSimpleRepMaxSfxContentDefinedChunker is RepMaxSfxCDC with the identity
// substitution box, mirroring github.com/buildbarn/go-cdc.
func NewSimpleRepMaxSfxContentDefinedChunker(minSizeBytes, horizonSizeBytes int) ContentDefinedChunker {
	return NewRepMaxSfxContentDefinedChunker(nil, minSizeBytes, horizonSizeBytes)
}

// NewAsymmetricExtremumContentDefinedChunker returns an AE (Asymmetric Extremum)
// chunker, mirroring github.com/buildbarn/go-cdc. Content-defined chunks are
// [minSizeBytes+1, maxSizeBytes]. Uses no Gear table. Backed by cdc/aecdc.
func NewAsymmetricExtremumContentDefinedChunker(minSizeBytes, maxSizeBytes int) ContentDefinedChunker {
	return &contentDefinedChunker{
		peek: maxSizeBytes,
		make: func(r io.Reader) rollinghash.Chunker {
			return aecdc.New(r, minSizeBytes, maxSizeBytes)
		},
	}
}

// NewSimpleAsymmetricExtremumContentDefinedChunker is identical to
// NewAsymmetricExtremumContentDefinedChunker.
func NewSimpleAsymmetricExtremumContentDefinedChunker(minSizeBytes, maxSizeBytes int) ContentDefinedChunker {
	return NewAsymmetricExtremumContentDefinedChunker(minSizeBytes, maxSizeBytes)
}
