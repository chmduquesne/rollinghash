package restic

import (
	"crypto/rand"
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/rabinkarp64"
)

const (
	kib = 1024
	mib = 1024 * kib

	// MinSize and MaxSize are restic/chunker's default chunk-size bounds.
	MinSize = 512 * kib
	MaxSize = 8 * mib

	// windowSize is the Rabin fingerprint window; it is fixed at 64, matching
	// restic/chunker.
	windowSize = 64

	// defaultAverageBits targets a 2^20-byte (1 MiB) average chunk, restic's
	// default.
	defaultAverageBits = 20
)

// Pol is restic/chunker's polynomial type. It is an alias for rabinkarp64.Pol,
// which carries the same GF(2) arithmetic and the same JSON encoding, so a
// polynomial read from a restic repository config round-trips unchanged.
type Pol = rabinkarp64.Pol

// RandomPolynomial returns a new random irreducible polynomial of degree 53,
// read from the system CSPRNG. It mirrors restic/chunker's RandomPolynomial.
func RandomPolynomial() (Pol, error) { return rabinkarp64.DerivePolynomial(rand.Reader) }

// DerivePolynomial returns an irreducible polynomial of degree 53 derived from
// source. It mirrors restic/chunker's DerivePolynomial.
func DerivePolynomial(source io.Reader) (Pol, error) { return rabinkarp64.DerivePolynomial(source) }

// Chunk is one content-defined chunk, mirroring restic/chunker.Chunk. Cut is
// the Rabin fingerprint of the 64-byte window ending at the boundary; it is 0
// for a final chunk whose whole stream is shorter than the window (see the
// package doc).
type Chunk struct {
	Start  uint
	Length uint
	Cut    uint64
	Data   []byte
}

// option configures a Chunker, mirroring restic/chunker's option type.
type option func(*Chunker)

// WithBoundaries sets custom min and max chunk-size bounds.
func WithBoundaries(min, max uint) option {
	return func(c *Chunker) { c.min, c.max = min, max }
}

// WithAverageBits sets the target average chunk size to 2^averageBits bytes:
// a boundary is cut where the low averageBits of the Rabin fingerprint are 0.
// The default is 20.
func WithAverageBits(averageBits int) option {
	return func(c *Chunker) { c.avgBits = averageBits }
}

// WithBuffer supplies a working buffer to reuse across streams. restic/chunker
// uses it as the reader buffer; here it sets the hashing batch size (a
// throughput knob only — it does not affect chunk boundaries). It is adopted
// when its capacity is at least the window size.
func WithBuffer(buf []byte) option {
	return func(c *Chunker) { c.buf = buf }
}

// Chunker splits an io.Reader into content-defined chunks with the same Rabin
// fingerprint splitter as restic/chunker, producing byte-identical boundaries.
// It wraps rollinghash.NewChunker over a rabinkarp64 hash.
type Chunker struct {
	it      rollinghash.Chunker
	rd      io.Reader
	pol     Pol
	min     uint
	max     uint
	avgBits int
	buf     []byte
	eofSeen bool
}

// New returns a Chunker that reads from rd and cuts with polynomial pol (obtain
// one from RandomPolynomial, DerivePolynomial, or a restic repository config).
// It mirrors restic/chunker.New. pol must be a non-zero polynomial of degree at
// most 53; New panics otherwise.
func New(rd io.Reader, pol Pol, opts ...option) *Chunker {
	c := &Chunker{}
	c.init(rd, pol, opts)
	return c
}

// NewWithBoundaries is New with WithBoundaries(min, max). Deprecated, kept for
// parity with restic/chunker.
func NewWithBoundaries(rd io.Reader, pol Pol, min, max uint) *Chunker {
	return New(rd, pol, WithBoundaries(min, max))
}

func (c *Chunker) init(rd io.Reader, pol Pol, opts []option) {
	c.rd = rd
	c.pol = pol
	c.min = MinSize
	c.max = MaxSize
	c.avgBits = defaultAverageBits
	c.buf = nil
	for _, opt := range opts {
		opt(c)
	}
	c.build()
}

func (c *Chunker) build() {
	if c.pol == 0 || c.pol.Deg() > 53 {
		panic("restic: polynomial must be non-zero and of degree <= 53 (use RandomPolynomial)")
	}
	h := rabinkarp64.NewFromPol(c.pol)
	mask := uint64(1)<<uint(c.avgBits) - 1
	minMax := rollinghash.WithBoundaries(int(c.min), int(c.max))
	if n := cap(c.buf); n >= windowSize {
		c.it = rollinghash.NewChunker(c.rd, h, windowSize, mask, minMax, rollinghash.WithBatchSize(n))
	} else {
		c.it = rollinghash.NewChunker(c.rd, h, windowSize, mask, minMax)
	}
	c.eofSeen = false
}

// Reset reinitialises the Chunker to split rd from the start with polynomial
// pol, keeping the working buffer. It mirrors restic/chunker.Chunker.Reset.
func (c *Chunker) Reset(rd io.Reader, pol Pol, opts ...option) {
	all := make([]option, 0, len(opts)+1)
	if c.buf != nil {
		all = append(all, WithBuffer(c.buf))
	}
	all = append(all, opts...)
	c.init(rd, pol, all)
}

// ResetWithBoundaries is Reset with WithBoundaries(min, max). Deprecated, kept
// for parity with restic/chunker.
func (c *Chunker) ResetWithBoundaries(rd io.Reader, pol Pol, min, max uint) {
	c.Reset(rd, pol, WithBoundaries(min, max))
}

// SetAverageBits changes the target average chunk size to 2^averageBits bytes.
// Like restic/chunker's method it is deprecated; prefer WithAverageBits at
// construction. It rebuilds the underlying chunker, so call it before the first
// Next — a mid-stream call restarts boundary detection from the reader's
// current position.
func (c *Chunker) SetAverageBits(averageBits int) {
	c.avgBits = averageBits
	c.build()
}

// Next returns the position and length of the next chunk of data, appending the
// chunk bytes to data (which may be nil). The final chunk is returned with a
// nil error; every call after that returns io.EOF. A reader error is returned
// as-is. It mirrors restic/chunker.Chunker.Next.
func (c *Chunker) Next(data []byte) (Chunk, error) {
	data = data[:0]
	if c.eofSeen {
		return Chunk{}, io.EOF
	}
	if !c.it.Next() {
		c.eofSeen = true
		if err := c.it.Err(); err != nil {
			return Chunk{}, err
		}
		return Chunk{}, io.EOF
	}
	b := c.it.Bytes()
	data = append(data, b...)
	return Chunk{
		Start:  uint(c.it.Offset()),
		Length: uint(len(b)),
		Cut:    c.it.Sum(),
		Data:   data,
	}, nil
}
