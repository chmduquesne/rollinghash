package main

import (
	"context"
	"io"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/bozo32"
	"github.com/chmduquesne/rollinghash/v4/bozo64"
	"github.com/chmduquesne/rollinghash/v4/buzhash32"
	"github.com/chmduquesne/rollinghash/v4/buzhash64"
	"github.com/chmduquesne/rollinghash/v4/cdc/aecdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/fastcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/jumpchunker"
	"github.com/chmduquesne/rollinghash/v4/cdc/maxcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/maxpcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/ramcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/repmaxcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/repmaxsfxcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/ultracdc"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
	"github.com/chmduquesne/rollinghash/v4/rabinkarp64"

	chunkers "github.com/PlakarKorp/go-cdc-chunkers"
	_ "github.com/PlakarKorp/go-cdc-chunkers/chunkers/fastcdc"
	_ "github.com/PlakarKorp/go-cdc-chunkers/chunkers/jc"
	_ "github.com/PlakarKorp/go-cdc-chunkers/chunkers/ultracdc"
)

// namedChunker is one row of the comparison: a label and a factory that turns a
// buffer into its chunk list.
type namedChunker struct {
	name string
	fn   chunkFunc
}

// chunkEvery is how often the drain loops check for context cancellation.
const chunkEvery = 512

// rollDrain streams r through a rollinghash.Chunker, aborting if ctx is
// cancelled.
func rollDrain(newC func(io.Reader) rollinghash.Chunker) chunkFunc {
	return func(ctx context.Context, r io.Reader, yield func([]byte)) error {
		c := newC(r)
		for i := 0; c.Next(); i++ {
			yield(c.Bytes())
			if i%chunkEvery == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
		}
		return c.Err()
	}
}

// plakarDrain streams r through a go-cdc-chunkers *Chunker.
func plakarDrain(algo string, o sizeOpts) chunkFunc {
	return func(ctx context.Context, r io.Reader, yield func([]byte)) error {
		c, err := chunkers.NewChunker(algo, r, &chunkers.ChunkerOpts{
			MinSize: o.min, NormalSize: o.normal, MaxSize: o.max,
		})
		if err != nil {
			return err
		}
		for i := 0; ; i++ {
			b, err := c.Next()
			if len(b) > 0 {
				yield(b)
			}
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if i%chunkEvery == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
		}
	}
}

// registry returns every algorithm to compare, this repo's first then the real
// go-cdc-chunkers ones, all parametrised toward the same size target.
func registry(o sizeOpts) []namedChunker {
	g := gearhash64.New
	// maxpcdc's chunk size scales super-linearly with its window; this mapping
	// is calibrated on random data and undershoots on low-entropy inputs, so its
	// avg (shown in the table) can land below target — read its saved% with that
	// in mind.
	maxpWin := max(1, o.normal*870/8192)

	return []namedChunker{
		{"parent-gear", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return rollinghash.NewChunker(r, gearhash64.New(), 56, uint64(o.normal-1),
				rollinghash.WithBoundaries(o.min, o.max))
		})},
		{"parent-buzhash", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return rollinghash.NewChunker(r, buzhash64.New(), 56, uint64(o.normal-1),
				rollinghash.WithBoundaries(o.min, o.max))
		})},
		{"parent-buzhash32", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return rollinghash.NewChunker(r, buzhash32.New(), 56, uint64(o.normal-1),
				rollinghash.WithBoundaries(o.min, o.max))
		})},
		{"parent-bozo32", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return rollinghash.NewChunker(r, bozo32.New(), 56, uint64(o.normal-1),
				rollinghash.WithBoundaries(o.min, o.max))
		})},
		{"parent-bozo64", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return rollinghash.NewChunker(r, bozo64.New(), 56, uint64(o.normal-1),
				rollinghash.WithBoundaries(o.min, o.max))
		})},
		{"parent-rabinkarp64", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return rollinghash.NewChunker(r, rabinkarp64.New(), 56, uint64(o.normal-1),
				rollinghash.WithBoundaries(o.min, o.max))
		})},
		{"fastcdc", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return fastcdc.New(r, g(), o.min, o.normal, o.max)
		})},
		{"jumpchunker", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return jumpchunker.New(r, g(), o.normal, o.min, o.max)
		})},
		{"ultracdc", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return ultracdc.New(r, o.min, o.normal, o.max)
		})},
		{"maxcdc", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return maxcdc.New(r, g(), o.normal/2, o.normal*13/8)
		})},
		{"repmaxcdc", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return repmaxcdc.New(r, g(), o.normal*3/4, o.normal)
		})},
		{"repmaxsfxcdc", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return repmaxsfxcdc.New(r, o.normal*3/4, o.normal)
		})},
		{"aecdc", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return aecdc.New(r, o.normal*15/16, o.max)
		})},
		{"ramcdc", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return ramcdc.New(r, o.normal-256, o.max)
		})},
		{"maxpcdc", rollDrain(func(r io.Reader) rollinghash.Chunker {
			return maxpcdc.New(r, maxpWin, o.max)
		})},

		// go-cdc-chunkers' spec (non-legacy) implementations — the legacy
		// aliases ("fastcdc", "ultracdc") clamp size opts and are misleading
		// above their 8/64 KiB defaults.
		{"plakar/fastcdc-v1.0.0", plakarDrain("fastcdc-v1.0.0", o)},
		{"plakar/jc-v1.1.0", plakarDrain("jc-v1.1.0", o)},
		{"plakar/ultracdc-v1.0.0", plakarDrain("ultracdc-v1.0.0", o)},
	}
}
