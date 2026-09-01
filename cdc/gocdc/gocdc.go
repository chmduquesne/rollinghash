// Package gocdc is a drop-in replacement for the consumer API of
// PlakarKorp/go-cdc-chunkers. Its NewChunker/NewChunkerBuffer, *Chunker,
// ChunkerOpts, ErrMinSize/ErrMaxSize/ErrNormalSize, and the Next/Split/Copy
// methods match that package's signatures, and it produces byte-identical chunk
// boundaries for the "fastcdc", "ultracdc", and "jc" families (including the
// "-v1.0.0" / "-v1.1.0" variants). Migrating is a one-line change:
//
//	import chunkers "github.com/chmduquesne/rollinghash/v4/cdc/gocdc"
//
// Keyed FastCDC ("kfastcdc") is not supported; those names return an error.
// Callers who want the typed, idiomatic API should use the cdc/fastcdc,
// cdc/ultracdc, and cdc/jumpchunker packages directly.
package gocdc

import (
	"errors"
	"fmt"
	"io"

	"github.com/chmduquesne/rollinghash/v4/cdc/fastcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/plakar"
	"github.com/chmduquesne/rollinghash/v4/cdc/jumpchunker"
	"github.com/chmduquesne/rollinghash/v4/cdc/ultracdc"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
)

// Validation errors, mirroring go-cdc-chunkers.
var (
	ErrNormalSize = errors.New("NormalSize must be a power of two, 64B <= NormalSize <= 1GB")
	ErrMinSize    = errors.New("MinSize must be 64B <= MinSize <= 1GB and MinSize < NormalSize")
	ErrMaxSize    = errors.New("MaxSize must be 64B <= MaxSize <= 1GB and MaxSize > NormalSize")
)

// ChunkerOpts mirrors chunkers.ChunkerOpts. A zero size field takes the
// selected algorithm's default. Key must be nil (keyed variants are
// unsupported).
type ChunkerOpts struct {
	MinSize    int
	MaxSize    int
	NormalSize int
	Key        []byte
}

// Chunker mirrors *chunkers.Chunker: an iterator over content-defined chunks.
type Chunker struct {
	it                        cutIter
	minSize, maxSize, normalS int
}

type cutIter interface {
	Next() bool
	Bytes() []byte
	Offset() int
	Err() error
	Reset(io.Reader)
}

// NewChunker returns a Chunker that reads from reader and cuts with the named
// algorithm. Recognized names: "fastcdc", "fastcdc-v1.0.0", "ultracdc",
// "ultracdc-v1.0.0", "jc", "jc-v1.0.0", "jc-v1.1.0".
func NewChunker(algorithm string, reader io.Reader, opts *ChunkerOpts) (*Chunker, error) {
	return newChunker(algorithm, reader, opts, nil)
}

// NewChunkerBuffer is like NewChunker but adopts buf as the working buffer when
// its capacity is large enough (roughly 2*MaxSize), for reuse across streams.
func NewChunkerBuffer(algorithm string, reader io.Reader, opts *ChunkerOpts, buf []byte) (*Chunker, error) {
	return newChunker(algorithm, reader, opts, buf)
}

func newChunker(algorithm string, reader io.Reader, opts *ChunkerOpts, buf []byte) (*Chunker, error) {
	var o ChunkerOpts
	if opts != nil {
		o = *opts
	}
	if o.Key != nil {
		return nil, fmt.Errorf("gocdc: %q: keyed variants are not supported", algorithm)
	}

	switch algorithm {
	case "fastcdc", "fastcdc-v1.0.0":
		o := withDefaults(o, 2*1024, 8*1024, 64*1024)
		if err := validate(o, true); err != nil {
			return nil, err
		}
		maskS, maskL := plakar.FastCDCSetupMasks(toPlakar(o), algorithm == "fastcdc")
		c := fastcdc.New(reader, gearhash64.NewFromUint64Array(plakar.GearTable),
			o.MinSize, o.NormalSize, o.MaxSize,
			fastcdc.WithMasks(maskS, maskL), fastcdc.WithBuffer(buf))
		return &Chunker{it: c, minSize: o.MinSize, maxSize: o.MaxSize, normalS: o.NormalSize}, nil

	case "jc", "jc-v1.0.0", "jc-v1.1.0":
		o := withDefaults(o, 2*1024, 8*1024, 64*1024)
		if err := validate(o, false); err != nil {
			return nil, err
		}
		// jc-v1.1.0 (newSpecJC) sets legacy=true too: it keeps the hardcoded
		// masks and only drops the sub-NormalSize early return.
		maskC, _, jumpLen := plakar.JCSetup(toPlakar(o), algorithm == "jc" || algorithm == "jc-v1.1.0")
		jopts := []jumpchunker.Option{
			jumpchunker.WithJumpMask(maskC, jumpLen),
			jumpchunker.WithBuffer(buf),
		}
		if algorithm == "jc-v1.1.0" {
			jopts = append(jopts, jumpchunker.WithSpecFaithful())
		}
		c := jumpchunker.New(reader, gearhash64.NewFromUint64Array(plakar.GearTable),
			o.NormalSize, o.MinSize, o.MaxSize, jopts...)
		return &Chunker{it: c, minSize: o.MinSize, maxSize: o.MaxSize, normalS: o.NormalSize}, nil

	case "ultracdc", "ultracdc-v1.0.0":
		o := withDefaults(o, 2*1024, 10*1024, 64*1024)
		if err := validate(o, false); err != nil {
			return nil, err
		}
		uopts := []ultracdc.Option{ultracdc.WithBuffer(buf)}
		if algorithm == "ultracdc-v1.0.0" {
			uopts = append(uopts, ultracdc.WithSpecFaithful())
		}
		c := ultracdc.New(reader, o.MinSize, o.NormalSize, o.MaxSize, uopts...)
		return &Chunker{it: c, minSize: o.MinSize, maxSize: o.MaxSize, normalS: o.NormalSize}, nil

	case "kfastcdc", "kfastcdc-v1.0.0":
		return nil, fmt.Errorf("gocdc: %q: keyed FastCDC is not supported", algorithm)
	default:
		return nil, fmt.Errorf("gocdc: unknown algorithm %q", algorithm)
	}
}

func withDefaults(o ChunkerOpts, minv, normal, maxv int) ChunkerOpts {
	if o.MinSize == 0 {
		o.MinSize = minv
	}
	if o.NormalSize == 0 {
		o.NormalSize = normal
	}
	if o.MaxSize == 0 {
		o.MaxSize = maxv
	}
	return o
}

func validate(o ChunkerOpts, wantPow2 bool) error {
	const gib = 1 << 30
	if o.NormalSize < 64 || o.NormalSize > gib || (wantPow2 && o.NormalSize&(o.NormalSize-1) != 0) {
		return ErrNormalSize
	}
	if o.MinSize < 64 || o.MinSize > gib || o.MinSize >= o.NormalSize {
		return ErrMinSize
	}
	if o.MaxSize < 64 || o.MaxSize > gib || o.MaxSize <= o.NormalSize {
		return ErrMaxSize
	}
	return nil
}

func toPlakar(o ChunkerOpts) plakar.Opts {
	return plakar.Opts{MinSize: o.MinSize, NormalSize: o.NormalSize, MaxSize: o.MaxSize}
}

// Next returns the next chunk. The final chunk is returned together with
// io.EOF; once the stream is drained Next returns (nil, io.EOF). A non-nil,
// non-EOF error is a reader error.
func (c *Chunker) Next() ([]byte, error) {
	if !c.it.Next() {
		if err := c.it.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	b := c.it.Bytes()
	if len(b) < c.minSize {
		return b, io.EOF // plakar signals EOF alongside a short final chunk
	}
	return b, nil
}

// Split calls callback for every chunk with its stream offset and length.
func (c *Chunker) Split(callback func(offset, length uint, chunk []byte) error) error {
	for c.it.Next() {
		b := c.it.Bytes()
		if err := callback(uint(c.it.Offset()), uint(len(b)), b); err != nil {
			return err
		}
	}
	return c.it.Err()
}

// Reset rewinds the Chunker to consume reader from the start, keeping its
// buffer and configuration.
func (c *Chunker) Reset(reader io.Reader) { c.it.Reset(reader) }

// MinSize, MaxSize and NormalSize report the effective bounds (after defaults).
func (c *Chunker) MinSize() int    { return c.minSize }
func (c *Chunker) MaxSize() int    { return c.maxSize }
func (c *Chunker) NormalSize() int { return c.normalS }

// Copy writes every chunk to dst in order and returns the total byte count.
func (c *Chunker) Copy(dst io.Writer) (int64, error) {
	var total int64
	for c.it.Next() {
		n, err := dst.Write(c.it.Bytes())
		total += int64(n)
		if err != nil {
			return total, err
		}
	}
	return total, c.it.Err()
}
