package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/pprof"
	"time"

	rollinghash "github.com/chmduquesne/rollinghash/v4"
	"github.com/chmduquesne/rollinghash/v4/adler32"
	"github.com/chmduquesne/rollinghash/v4/bozo32"
	"github.com/chmduquesne/rollinghash/v4/bozo64"
	"github.com/chmduquesne/rollinghash/v4/buzhash32"
	"github.com/chmduquesne/rollinghash/v4/buzhash64"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
	"github.com/chmduquesne/rollinghash/v4/rabinkarp64"
	humanize "github.com/dustin/go-humanize"
)

const (
	KiB = 1024
	MiB = 1024 * KiB
	GiB = 1024 * MiB

	clearscreen = "\033[2J\033[1;1H"
	clearline   = "\x1b[2K"

	window = 48
)

var hashes = map[string]func() rollinghash.Hash{
	"adler32":     func() rollinghash.Hash { return adler32.New() },
	"bozo32":      func() rollinghash.Hash { return bozo32.New() },
	"bozo64":      func() rollinghash.Hash { return bozo64.New() },
	"buzhash32":   func() rollinghash.Hash { return buzhash32.New() },
	"buzhash64":   func() rollinghash.Hash { return buzhash64.New() },
	"gearhash64":  func() rollinghash.Hash { return gearhash64.New() },
	"rabinkarp64": func() rollinghash.Hash { return rabinkarp64.New() },
}

func genMasks() []uint64 {
	res := make([]uint64, 64)
	ones := ^uint64(0)
	for i := range res {
		res[i] = ones >> uint(63-i)
	}
	return res
}

// prefetchReader wraps an io.Reader and reads the next chunk in a background
// goroutine, keeping one chunk ahead so disk I/O and hashing can overlap.
// It uses two fixed buffers alternating in a ping-pong fashion.
type prefetchReader struct {
	forward chan []byte
	free    chan []byte
	cur     []byte
	pos     int
	err     error // set before close(forward); safe to read after !ok receive
}

func newPrefetchReader(r io.Reader, chunkSize int) *prefetchReader {
	pr := &prefetchReader{
		forward: make(chan []byte, 1),
		free:    make(chan []byte, 2),
	}
	pr.free <- make([]byte, chunkSize)
	pr.free <- make([]byte, chunkSize)
	go func() {
		defer close(pr.forward)
		for {
			buf := <-pr.free
			n, err := io.ReadFull(r, buf)
			if n > 0 {
				pr.forward <- buf[:n]
			}
			if err != nil {
				pr.err = err
				return
			}
		}
	}()
	return pr
}

func (pr *prefetchReader) Read(p []byte) (int, error) {
	for pr.pos >= len(pr.cur) {
		if pr.cur != nil {
			pr.free <- pr.cur[:cap(pr.cur)] // return buffer to producer
		}
		cur, ok := <-pr.forward
		if !ok {
			if pr.err != nil && pr.err != io.ErrUnexpectedEOF {
				return 0, pr.err
			}
			return 0, io.EOF
		}
		pr.cur = cur
		pr.pos = 0
	}
	n := copy(p, pr.cur[pr.pos:])
	pr.pos += n
	return n, nil
}

func main() {
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	dostats := flag.Bool("stats", false, "Do some stats about the rolling sum")
	size := flag.String("size", "256M", "How much data to read")
	hashName := flag.String("hash", "buzhash64", "rolling hash to use: adler32, bozo32, bozo64, buzhash32, buzhash64, gearhash64, rabinkarp64")
	input := flag.String("input", "/dev/urandom", "file to read from")
	flag.Parse()

	newHash, ok := hashes[*hashName]
	if !ok {
		log.Fatalf("unknown hash %q: choose from adler32, bozo32, bozo64, buzhash32, buzhash64, gearhash64, rabinkarp64", *hashName)
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal(err)
		}
		defer pprof.StopCPUProfile()
	}

	fileSize, err := humanize.ParseBytes(*size)
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.Open(*input)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	masks := genMasks()
	hits := make(map[uint64]uint64, len(masks))
	for _, m := range masks {
		hits[m] = 0
	}

	const bufsize = 16 * MiB
	pr := newPrefetchReader(io.LimitReader(f, int64(fileSize)), bufsize)
	s := rollinghash.NewBatchRoller(pr, newHash(), window, rollinghash.WithBufferSize(bufsize))

	n := uint64(0)
	nextPrint := uint64(bufsize)
	t := time.Now()

	for s.Next() {
		sums := s.Sums()
		if *dostats {
			for _, sum := range sums {
				for _, m := range masks {
					if sum&m == m {
						hits[m]++
					} else {
						break
					}
				}
			}
		}
		n += uint64(len(sums))
		if n >= nextPrint {
			status := fmt.Sprintf("Byte count: %s", humanize.Bytes(n))
			if *dostats {
				fmt.Print(clearscreen)
				fmt.Println(status)
				for i, m := range masks {
					frequency := "NaN"
					if hits[m] != 0 {
						// Float division then round: integer n/hits floors,
						// so a perfectly balanced hash (true freq ~2^k, e.g.
						// 15.9986) would misleadingly print 2^k-1 (15).
						frequency = humanize.Bytes(uint64(float64(n)/float64(hits[m]) + 0.5))
					}
					fmt.Printf("0x%016x (%02d bits): every %s\n", m, i+1, frequency)
				}
			} else {
				fmt.Print(clearline)
				fmt.Print(status)
				fmt.Print("\r")
			}
			nextPrint += bufsize
		}
	}
	if err := s.Err(); err != nil {
		log.Fatal(err)
	}

	duration := time.Since(t)
	fmt.Printf("Rolled %s of data in %v (%s/s).\n",
		humanize.Bytes(n),
		duration,
		humanize.Bytes(uint64(float64(n)/duration.Seconds())),
	)
}
