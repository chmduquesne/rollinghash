package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/bits"
	"math/rand"
	"os"
	"runtime/pprof"
	"sort"
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
	KiB    = 1024
	MiB    = 1024 * KiB
	window = 56
)

var hashConstructors = map[string]func() rollinghash.Hash{
	"adler32":     func() rollinghash.Hash { return adler32.New() },
	"bozo32":      func() rollinghash.Hash { return bozo32.New() },
	"bozo64":      func() rollinghash.Hash { return bozo64.New() },
	"buzhash32":   func() rollinghash.Hash { return buzhash32.New() },
	"buzhash64":   func() rollinghash.Hash { return buzhash64.New() },
	"gearhash64":  func() rollinghash.Hash { return gearhash64.New() },
	"rabinkarp64": func() rollinghash.Hash { return rabinkarp64.New() },
}

// randReader generates pseudo-random bytes via math/rand (much faster than /dev/urandom).
type randReader struct {
	rng *rand.Rand
}

func (r *randReader) Read(p []byte) (int, error) {
	n := 0
	for n+8 <= len(p) {
		v := r.rng.Uint64()
		p[n] = byte(v); p[n+1] = byte(v >> 8); p[n+2] = byte(v >> 16); p[n+3] = byte(v >> 24)
		p[n+4] = byte(v >> 32); p[n+5] = byte(v >> 40); p[n+6] = byte(v >> 48); p[n+7] = byte(v >> 56)
		n += 8
	}
	if n < len(p) {
		v := r.rng.Uint64()
		for n < len(p) {
			p[n] = byte(v)
			v >>= 8
			n++
		}
	}
	return len(p), nil
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
			pr.free <- pr.cur[:cap(pr.cur)]
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

// dataSource returns an io.Reader limited to size bytes and a cleanup function.
// When input is empty, data is generated in-process via math/rand.
func dataSource(size int64, input string) (io.Reader, func()) {
	if input == "" {
		rng := rand.New(rand.NewSource(0))
		pr := newPrefetchReader(io.LimitReader(&randReader{rng: rng}, size), 16*MiB)
		return pr, func() {}
	}
	f, err := os.Open(input)
	if err != nil {
		log.Fatal(err)
	}
	pr := newPrefetchReader(io.LimitReader(f, size), 16*MiB)
	return pr, func() {
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
}

// rollSpeed benchmarks the Roll interface using the concrete type H so that
// the compiler can inline Roll (see README: interface dispatch prevents inlining).
func rollSpeed[H interface {
	Write([]byte) (int, error)
	Roll(byte)
}](r io.Reader, h H) float64 {
	buf := make([]byte, 16*MiB)
	n, err := io.ReadFull(r, buf[:window])
	if n < window {
		log.Fatalf("data source too small to initialize window (%d bytes)", n)
	}
	if err != nil && err != io.ErrUnexpectedEOF {
		log.Fatal(err)
	}
	h.Write(buf[:window])

	t := time.Now()
	rolled := int64(0)
	for {
		n, err := r.Read(buf)
		for i := range n {
			h.Roll(buf[i])
		}
		rolled += int64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	return float64(rolled) / time.Since(t).Seconds() / 1e6
}

func rollerMBps(r io.Reader, newHash func() rollinghash.Hash) float64 {
	switch h := newHash().(type) {
	case *adler32.Adler32:
		return rollSpeed(r, h)
	case *bozo32.Bozo32:
		return rollSpeed(r, h)
	case *bozo64.Bozo64:
		return rollSpeed(r, h)
	case *buzhash32.Buzhash32:
		return rollSpeed(r, h)
	case *buzhash64.Buzhash64:
		return rollSpeed(r, h)
	case *gearhash64.GearHash64:
		return rollSpeed(r, h)
	case *rabinkarp64.RabinKarp64:
		return rollSpeed(r, h)
	default:
		panic("rollstat: unknown hash type")
	}
}

// cycleReader repeats a fixed buffer until remaining bytes are exhausted.
// This keeps the working set in cache, matching Go benchmark conditions.
type cycleReader struct {
	data      []byte
	pos       int
	remaining int64
}

func (r *cycleReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	if r.pos >= len(r.data) {
		r.pos = 0
	}
	n := copy(p, r.data[r.pos:])
	if int64(n) > r.remaining {
		n = int(r.remaining)
	}
	r.pos += n
	r.remaining -= int64(n)
	return n, nil
}

// speedDataSource returns an in-memory cycling reader for speed measurements
// when no input file is given. A 1 MiB buffer stays hot in L3 cache,
// matching Go benchmark conditions and eliminating generation overhead.
func speedDataSource(size int64, input string) (io.Reader, int64, func()) {
	if input != "" {
		r, cleanup := dataSource(size, input)
		return r, size, cleanup
	}
	buf := make([]byte, 1*MiB)
	rr := &randReader{rng: rand.New(rand.NewSource(0))}
	rr.Read(buf)
	return &cycleReader{data: buf, remaining: size}, size, func() {}
}

func cmdSpeed(newHash func() rollinghash.Hash, hashName, input, iface string, size int64) {
	r, actualSize, cleanup := speedDataSource(size, input)
	defer cleanup()

	var mbps float64
	switch iface {
	case "roll":
		mbps = rollerMBps(r, newHash)
	case "batchroll":
		s := rollinghash.NewBatchRoller(r, newHash(), window, rollinghash.WithBufferSize(16*MiB))
		t := time.Now()
		for s.Next() {
		}
		if err := s.Err(); err != nil {
			log.Fatal(err)
		}
		mbps = float64(actualSize) / time.Since(t).Seconds() / 1e6
	case "chunk":
		c := rollinghash.NewChunker(r, newHash(), window, 0xff)
		t := time.Now()
		for c.Next() {
		}
		if err := c.Err(); err != nil {
			log.Fatal(err)
		}
		mbps = float64(actualSize) / time.Since(t).Seconds() / 1e6
	default:
		log.Fatalf("unknown interface %q: choose roll, batchroll, or chunk", iface)
	}

	src := "rand"
	if input != "" {
		src = input
	}
	fmt.Printf("Hash: %-12s  Interface: %-10s  Window: %d  Data: %s  Source: %s\n",
		hashName, iface, window, humanize.Bytes(uint64(actualSize)), src)
	fmt.Printf("%.0f MB/s\n", mbps)
}

func cmdAll(input string, size int64) {
	names := make([]string, 0, len(hashConstructors))
	for name := range hashConstructors {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("=== %s ===\n\n", name)
		newHash := hashConstructors[name]
		for _, iface := range []string{"roll", "batchroll", "chunk"} {
			cmdSpeed(newHash, name, input, iface, size)
		}
		fmt.Println()
		cmdStat(newHash, name, input, size)
		fmt.Println()
	}
}

func cmdStat(newHash func() rollinghash.Hash, hashName, input string, size int64) {
	r, cleanup := dataSource(size, input)
	defer cleanup()

	// Use a byte-level histogram to derive per-bit frequencies efficiently:
	// 8 increments per sum instead of 64, with a small post-processing step.
	var byteCounts [8][256]uint64
	var tzHist [65]uint64 // index 64 is the special case sum==0
	var total uint64

	s := rollinghash.NewBatchRoller(r, newHash(), window, rollinghash.WithBufferSize(16*MiB))
	for s.Next() {
		for _, sum := range s.Sums() {
			byteCounts[0][sum&0xff]++
			byteCounts[1][(sum>>8)&0xff]++
			byteCounts[2][(sum>>16)&0xff]++
			byteCounts[3][(sum>>24)&0xff]++
			byteCounts[4][(sum>>32)&0xff]++
			byteCounts[5][(sum>>40)&0xff]++
			byteCounts[6][(sum>>48)&0xff]++
			byteCounts[7][sum>>56]++
			tzHist[bits.TrailingZeros64(sum)]++
			total++
		}
	}
	if err := s.Err(); err != nil {
		log.Fatal(err)
	}

	// Derive per-bit frequencies from the byte histograms.
	var bitCounts [64]uint64
	for b := range 8 {
		for v := range 256 {
			c := byteCounts[b][v]
			if c == 0 {
				continue
			}
			for i := range 8 {
				if (v>>i)&1 == 1 {
					bitCounts[b*8+i] += c
				}
			}
		}
	}

	src := "rand"
	if input != "" {
		src = input
	}
	fmt.Printf("Hash: %s   Window: %d   Data: %s   Source: %s   Positions: %s\n\n",
		hashName, window, humanize.Bytes(uint64(size)), src, humanize.Comma(int64(total)))

	fmt.Printf("Bit entropy per bit:\n")
	fmt.Printf("  %3s  %8s  %8s\n", "pos", "p(bit=1)", "H")
	minH, maxH, sumH := 1.0, 0.0, 0.0
	for i, c := range bitCounts {
		p := float64(c) / float64(total)
		var h float64
		if p > 0 && p < 1 {
			h = -p*math.Log2(p) - (1-p)*math.Log2(1-p)
		}
		warn := ""
		if h < 0.99 {
			warn = " !"
		}
		fmt.Printf("  %3d  %8.5f  %8.5f%s\n", i, p, h, warn)
		if h < minH {
			minH = h
		}
		if h > maxH {
			maxH = h
		}
		sumH += h
	}
	fmt.Printf("  summary: min=%.5f  max=%.5f  mean=%.5f\n", minH, maxH, sumH/64)

	fmt.Printf("\nTrailing zero decay (CDC chunk-size uniformity):\n")
	fmt.Printf("  %3s  %12s  %12s  %8s\n", "k", "observed", "expected", "ratio")
	for k := range 20 {
		exp := math.Pow(0.5, float64(k+1))
		if float64(total)*exp < 1000 {
			break
		}
		obs := float64(tzHist[k]) / float64(total)
		ratio := obs / exp
		warn := ""
		if math.Abs(ratio-1.0) > 0.05 {
			warn = " !"
		}
		fmt.Printf("  %3d  %12.6f  %12.6f  %8.4f%s\n", k, obs, exp, ratio, warn)
	}
}

func main() {
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	sizeStr := flag.String("size", "10G", "amount of data to process")
	hashName := flag.String("hash", "buzhash64", "hash: adler32, bozo32, bozo64, buzhash32, buzhash64, gearhash64, rabinkarp64")
	input := flag.String("input", "", "input file (default: generate via math/rand)")
	flag.Parse()

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

	size, err := humanize.ParseBytes(*sizeStr)
	if err != nil {
		log.Fatal(err)
	}

	newHash, ok := hashConstructors[*hashName]
	if !ok {
		log.Fatalf("unknown hash %q: choose from adler32, bozo32, bozo64, buzhash32, buzhash64, gearhash64, rabinkarp64", *hashName)
	}

	args := flag.Args()
	if len(args) == 0 {
		log.Fatal("usage: rollstat [--hash H] [--size N] [--input FILE] <speed|stat|all> [--interface roll|batchroll|chunk]")
	}

	switch args[0] {
	case "speed":
		fs := flag.NewFlagSet("speed", flag.ExitOnError)
		iface := fs.String("interface", "batchroll", "interface to benchmark: roll, batchroll, chunk")
		fs.Parse(args[1:])
		cmdSpeed(newHash, *hashName, *input, *iface, int64(size))
	case "stat":
		cmdStat(newHash, *hashName, *input, int64(size))
	case "all":
		cmdAll(*input, int64(size))
	default:
		log.Fatalf("unknown command %q: choose speed, stat, or all", args[0])
	}
}
