package boxo

import (
	"errors"
	"io"
)

// DefaultBlockSize is the chunk size the fixed-size splitter aims for, matching
// boxo/chunker. It is a var so callers that set it before constructing
// splitters get boxo's behaviour.
var DefaultBlockSize int64 = 1024 * 256

// Splitter reads bytes from a Reader and produces chunks (byte slices).
type Splitter interface {
	Reader() io.Reader
	NextBytes() ([]byte, error)
}

// SplitterGen creates a Splitter from a reader.
type SplitterGen func(r io.Reader) Splitter

// DefaultSplitter returns a fixed-size Splitter using DefaultBlockSize.
func DefaultSplitter(r io.Reader) Splitter {
	return NewSizeSplitter(r, DefaultBlockSize)
}

// SizeSplitterGen returns a SplitterGen for fixed-size chunks of the given size.
func SizeSplitterGen(size int64) SplitterGen {
	return func(r io.Reader) Splitter {
		return NewSizeSplitter(r, size)
	}
}

// Chan returns a channel of the chunks produced by s and a channel for the
// terminating error (io.EOF at a clean end).
func Chan(s Splitter) (<-chan []byte, <-chan error) {
	out := make(chan []byte)
	errs := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errs)
		for {
			b, err := s.NextBytes()
			if err != nil {
				errs <- err
				return
			}
			out <- b
		}
	}()
	return out, errs
}

// sizeSplitter is boxo's fixed-size splitter. It reads exactly size bytes per
// chunk, with a short final chunk, and no rolling hash.
type sizeSplitter struct {
	r    io.Reader
	size int
	err  error
}

// NewSizeSplitter returns a fixed-size Splitter with the given block size.
// As in boxo, size is taken modulo 2^32.
func NewSizeSplitter(r io.Reader, size int64) Splitter {
	return &sizeSplitter{r: r, size: int(uint32(size))}
}

func (ss *sizeSplitter) NextBytes() ([]byte, error) {
	if ss.err != nil {
		return nil, ss.err
	}
	buf := make([]byte, ss.size)
	n, err := io.ReadFull(ss.r, buf)
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			ss.err = io.EOF
			if n == 0 {
				return nil, io.EOF
			}
			return buf[:n], nil
		}
		if errors.Is(err, io.EOF) {
			ss.err = io.EOF
			return nil, io.EOF
		}
		ss.err = err
		return nil, err
	}
	return buf, nil
}

func (ss *sizeSplitter) Reader() io.Reader { return ss.r }
