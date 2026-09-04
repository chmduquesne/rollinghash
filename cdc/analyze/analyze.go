package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"math"
	"math/rand"
	"os"
	"sort"
	"time"
)

// chunkFunc streams data through a chunker, calling yield once per chunk. The
// slice passed to yield is only valid for that call. It aborts when ctx is
// cancelled. Both this repo's rollinghash.Chunker and go-cdc-chunkers'
// *chunkers.Chunker are wrapped into this shape by chunkers.go.
type chunkFunc func(ctx context.Context, r io.Reader, yield func(chunk []byte)) error

// resyncInputCap bounds the file resync re-chunks so a 1.5 GB member does not
// blow the time budget; the shared prefix is what the metric measures anyway.
const resyncInputCap = 128 << 20

// result is the full measurement of one algorithm over one dataset.
type result struct {
	algorithm string

	files      int
	totalBytes int64
	chunks     int

	uniqueBytes int64
	uniqueChunk int

	lengths      []int
	duration     time.Duration
	resyncShared float64 // -1 if not run
}

func (r *result) dedupRatio() float64 {
	if r.totalBytes == 0 {
		return 1
	}
	return float64(r.uniqueBytes) / float64(r.totalBytes)
}

func (r *result) savedPct() float64 { return 100 * (1 - r.dedupRatio()) }

func (r *result) throughputMBs() float64 {
	if r.duration == 0 {
		return 0
	}
	return float64(r.totalBytes) / 1e6 / r.duration.Seconds()
}

type distribution struct {
	min, p50, avg, p95, max int
	stddev                  float64
}

func (r *result) distribution() distribution {
	var d distribution
	if len(r.lengths) == 0 {
		return d
	}
	s := append([]int(nil), r.lengths...)
	sort.Ints(s)
	d.min, d.max = s[0], s[len(s)-1]
	d.p50 = s[len(s)*50/100]
	d.p95 = s[min(len(s)*95/100, len(s)-1)]

	var sum int64
	for _, l := range s {
		sum += int64(l)
	}
	mean := float64(sum) / float64(len(s))
	d.avg = int(mean)
	var sq float64
	for _, l := range s {
		x := float64(l) - mean
		sq += x * x
	}
	d.stddev = math.Sqrt(sq / float64(len(s)))
	return d
}

// measure chunks every file in order with one shared digest set, so duplication
// across the ordered versions of a dataset drives the dedup ratio. Files are
// streamed from disk and never fully held in memory.
func measure(ctx context.Context, algorithm string, cf chunkFunc, paths []string) (*result, error) {
	res := &result{algorithm: algorithm, files: len(paths), resyncShared: -1}
	seen := make(map[[32]byte]struct{})

	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		start := time.Now()
		err = cf(ctx, f, func(c []byte) {
			res.chunks++
			res.totalBytes += int64(len(c))
			res.lengths = append(res.lengths, len(c))
			d := sha256.Sum256(c)
			if _, ok := seen[d]; !ok {
				seen[d] = struct{}{}
				res.uniqueChunk++
				res.uniqueBytes += int64(len(c))
			}
		})
		res.duration += time.Since(start)
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

// resync measures boundary shift-resistance: chunk base, apply `edits` single-
// byte insertions to a copy, re-chunk, and return the fraction of the edited
// stream (walked in order) carried by chunks whose digest is also in base.
func resync(ctx context.Context, cf chunkFunc, basePath string, edits int, seed int64) (float64, error) {
	f, err := os.Open(basePath)
	if err != nil {
		return 0, err
	}
	base, err := io.ReadAll(io.LimitReader(f, resyncInputCap))
	f.Close()
	if err != nil {
		return 0, err
	}

	baseSet := make(map[[32]byte]struct{})
	if err := cf(ctx, bytes.NewReader(base), func(c []byte) {
		baseSet[sha256.Sum256(c)] = struct{}{}
	}); err != nil {
		return 0, err
	}

	edited := applyInsertions(base, edits, seed)
	var shared, total int64
	if err := cf(ctx, bytes.NewReader(edited), func(c []byte) {
		total += int64(len(c))
		if _, ok := baseSet[sha256.Sum256(c)]; ok {
			shared += int64(len(c))
		}
	}); err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	return float64(shared) / float64(total), nil
}

// applyInsertions returns a copy of data with n single-position insertions of
// one random byte. Insertions shift the tail — the hard case for re-sync.
func applyInsertions(data []byte, n int, seed int64) []byte {
	r := rand.New(rand.NewSource(seed))
	out := append([]byte(nil), data...)
	for range n {
		pos := 0
		if len(out) > 0 {
			pos = r.Intn(len(out))
		}
		b := byte(r.Intn(256))
		out = append(out[:pos], append([]byte{b}, out[pos:]...)...)
	}
	return out
}
