package duplicacy

import (
	"crypto/sha256"
	"encoding/binary"
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/buzhash64"
)

const (
	kib = 1024
	mib = 1024 * kib

	// DefaultAverageChunkSize, DefaultMinimumChunkSize and DefaultMaximumChunkSize
	// are the chunk-size parameters Duplicacy's CLI uses by default (the -chunk-size,
	// -min-chunk-size and -max-chunk-size flags of "duplicacy init"): a 4 MiB target
	// with the minimum at a quarter of it and the maximum at four times it.
	DefaultAverageChunkSize = 4 * mib
	DefaultMinimumChunkSize = 1 * mib
	DefaultMaximumChunkSize = 16 * mib
)

// DefaultChunkSeed is the seed Duplicacy uses to derive the buzhash table for an
// unencrypted repository: the literal string "duplicacy" (its DEFAULT_KEY). An
// encrypted repository instead uses the first 32 bytes of its random key
// material, stored in the repository config; pass those with WithChunkSeed.
var DefaultChunkSeed = []byte("duplicacy")

// deriveTable reproduces Duplicacy's ChunkMaker random-table construction: hash
// the seed with SHA-256, take the digest's first 32 bytes as four little-endian
// uint64s, then repeatedly re-hash the digest, 64 rounds in all for 256 entries.
func deriveTable(seed []byte) [256]uint64 {
	var table [256]uint64
	digest := sha256.Sum256(seed)
	for i := range 64 {
		for j := range 4 {
			table[4*i+j] = binary.LittleEndian.Uint64(digest[8*j : 8*j+8])
		}
		digest = sha256.Sum256(digest[:])
	}
	return table
}

// config holds the resolved parameters for a ChunkMaker.
type config struct {
	seed []byte
	avg  int
	min  int
	max  int
	buf  []byte
}

// Option configures a ChunkMaker.
type Option func(*config)

// WithChunkSeed sets the seed used to derive the buzhash table. It must match
// the ChunkSeed of the Duplicacy repository whose boundaries you want to
// reproduce (see DefaultChunkSeed). The bytes are not retained.
func WithChunkSeed(seed []byte) Option {
	return func(c *config) { c.seed = append([]byte(nil), seed...) }
}

// WithAverageChunkSize sets the target average chunk size in bytes. It must be a
// power of two; CreateChunkMaker panics otherwise, matching Duplicacy. The
// boundary mask is averageChunkSize-1.
func WithAverageChunkSize(n int) Option { return func(c *config) { c.avg = n } }

// WithMinimumChunkSize sets the minimum chunk size in bytes. It doubles as the
// buzhash rolling-window length, exactly as in Duplicacy.
func WithMinimumChunkSize(n int) Option { return func(c *config) { c.min = n } }

// WithMaximumChunkSize sets the maximum chunk size in bytes: a boundary is
// forced there when no content-defined cut is found.
func WithMaximumChunkSize(n int) Option { return func(c *config) { c.max = n } }

// WithBuffer supplies a working buffer to reuse across streams. When its
// capacity exceeds the default hashing batch (~8*minimumChunkSize, capped at
// 16 MiB) it also raises that batch, which is a throughput knob only — it does
// not move boundaries. A larger batch amortises the per-call window priming
// BatchBoundaries does; returns diminish once the batch spills the last-level
// cache.
func WithBuffer(buf []byte) Option { return func(c *config) { c.buf = buf } }

// Chunk is one content-defined chunk. Duplicacy's own Chunk type carries only
// the bytes (and, later, their content hash); Start, Length and Hash are added
// here. Data aliases the maker's internal buffer and is only valid for the
// duration of the sendChunk call that received it; copy it if you need to keep
// it.
type Chunk struct {
	Start  int
	Length int
	// Hash is the buzhash of the 64-bit rolling window ending at the cut — the
	// value Duplicacy's ChunkMaker tests against its hashMask. It is the mask
	// hit value at a content-defined boundary; at a boundary forced by the
	// maximum chunk size it is the window checksum there (which did not satisfy
	// the mask), and 0 for a final chunk shorter than the window.
	Hash uint64
	Data []byte
}

// ChunkMaker splits a stream of data into content-defined chunks with the same
// buzhash splitter as Duplicacy's ChunkMaker, producing byte-identical
// boundaries for a given seed and size triple. It wraps rollinghash.NewChunkWriter
// over a buzhash64 seeded with Duplicacy's table.
type ChunkMaker struct {
	w      rollinghash.ChunkWriter
	buf    []byte
	sent   int
	closed bool
}

// CreateChunkMaker returns a ChunkMaker. With no options it uses DefaultChunkSeed
// and the Default*ChunkSize triple. It mirrors Duplicacy's CreateFileChunkMaker.
// It panics if averageChunkSize is not a power of two, if minimumChunkSize is
// below 64 (the rolling window must fit), or if the sizes are not ordered
// min <= average <= max.
func CreateChunkMaker(opts ...Option) *ChunkMaker {
	c := &config{
		seed: DefaultChunkSeed,
		avg:  DefaultAverageChunkSize,
		min:  DefaultMinimumChunkSize,
		max:  DefaultMaximumChunkSize,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.avg < 1 || c.avg&(c.avg-1) != 0 {
		panic("duplicacy: averageChunkSize must be a power of two")
	}
	if c.min < 64 {
		panic("duplicacy: minimumChunkSize must be at least 64 (the buzhash window)")
	}
	if c.min > c.avg || c.avg > c.max {
		panic("duplicacy: sizes must be ordered minimumChunkSize <= averageChunkSize <= maximumChunkSize")
	}

	h := buzhash64.NewFromUint64Array(deriveTable(c.seed))
	mask := uint64(c.avg - 1)
	minMax := rollinghash.WithBoundaries(c.min, c.max)

	// The buzhash window equals minimumChunkSize, often megabytes.
	// BatchBoundaries re-primes the whole window on every call, so a batch
	// smaller than the window would re-hash it many times over: keep the
	// batch several windows wide so priming cost amortises and throughput
	// stays close to a continuous roll. ~8 windows is the knee (past ~16 MiB
	// the batch spills the last-level cache and it gets slower again); the
	// caller can override with a larger WithBuffer.
	batch := min(8*c.min, c.max, 16*mib)
	if n := cap(c.buf); n > batch {
		batch = n
	}

	// Chunk.Hash comes from ChunkWriter.Sum(), which the engine computes at
	// every cut anyway.
	m := &ChunkMaker{
		w:   rollinghash.NewChunkWriter(h, c.min, mask, minMax, rollinghash.WithBatchSize(batch)),
		buf: make([]byte, min(batch, 256*kib)),
	}
	if cap(c.buf) > len(m.buf) {
		m.buf = c.buf[:cap(c.buf)]
	}
	return m
}

// AddData feeds every byte of reader into the maker, calling sendChunk for each
// completed chunk in order. Like Duplicacy's maker.AddData it may be called
// repeatedly to chunk a sequence of readers as one continuous stream; call it a
// final time with a nil reader to flush the trailing bytes as a last, possibly
// short, chunk. After the nil-reader call the maker must be Reset before reuse.
//
// A read error (other than io.EOF) is returned as-is and leaves the maker
// unusable.
func (m *ChunkMaker) AddData(reader io.Reader, sendChunk func(Chunk)) error {
	if m.closed {
		return rollinghash.ErrClosed
	}
	if reader == nil {
		m.closed = true
		_ = m.w.Close()
		return m.drain(sendChunk)
	}
	for {
		n, err := reader.Read(m.buf)
		if n > 0 {
			if _, werr := m.w.Write(m.buf[:n]); werr != nil {
				return werr
			}
			if derr := m.drain(sendChunk); derr != nil {
				return derr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (m *ChunkMaker) drain(sendChunk func(Chunk)) error {
	for m.w.Next() {
		b := m.w.Bytes()
		sendChunk(Chunk{Start: m.sent, Length: len(b), Hash: m.w.Sum(), Data: b})
		m.sent += len(b)
	}
	return m.w.Err()
}

// Reset clears all buffered state so the maker can chunk a new stream, keeping
// its allocations and configuration. It also un-closes a maker whose AddData was
// last called with a nil reader.
func (m *ChunkMaker) Reset() {
	m.w.Reset()
	m.sent = 0
	m.closed = false
}
