package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/chmduquesne/rollinghash"
	_adler32 "github.com/chmduquesne/rollinghash/adler32"
	"github.com/chmduquesne/rollinghash/buzhash32"
	"github.com/chmduquesne/rollinghash/buzhash64"
	"github.com/chmduquesne/rollinghash/rabinkarp32"
	"github.com/cloudfoundry/bytefmt"
)

const (
	KiB = 1024
	MiB = 1024 * KiB
	GiB = 1024 * MiB
)

func printProgress(format string, args ...interface{}) {
	clearline := "\x1b[2K"
	formatted := fmt.Sprintf(format, args...)
	toPrint := fmt.Sprintf("%s%s%s", clearline, formatted, "\r")
	fmt.Print(toPrint)
}

func genMasks() (res []uint64) {
	res = make([]uint64, 64)
	allones := ^uint64(0) // 0xffffffffffffffff
	for i := 0; i < 64; i++ {
		res[i] = allones >> uint(63-i)
	}
	return
}

func hash2uint64(s []byte) (res uint64) {
	for _, b := range s {
		res <<= 8
		res |= uint64(b)
	}
	return
}

func main() {
	rollsum := flag.String("sum", "adler32", "adler32|rabinkarb32|buzhash32|buzhash64")
	dostats := flag.Bool("stats", false, "Do some stats about the rolling sum")
	flag.Parse()

	bufsize := 512 * KiB
	rbuf := make([]byte, bufsize)
	hbuf := make([]byte, 0, 8)
	t := time.Now()

	f, err := os.Open("/dev/urandom")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	io.ReadFull(f, rbuf)

	var roll rollinghash.Hash
	switch *rollsum {
	case "adler32":
		roll = _adler32.New()
	case "rabinkarp32":
		roll = rabinkarp32.New()
	case "buzhash32":
		roll = buzhash32.New()
	case "buzhash64":
		roll = buzhash64.New()
	default:
		log.Fatalf("%s: unrecognized checksum", *rollsum)
	}
	roll.Write(rbuf[:64])

	masks := genMasks()
	hits := make(map[uint64]uint64)
	for _, m := range masks {
		hits[m] = 0
	}

	n := uint64(0)
	k := 0
	for n < 256*MiB {
		if k >= bufsize {
			printProgress("bytes count: %s", bytefmt.ByteSize(n))
			io.ReadFull(f, rbuf)
			k = 0
		}
		roll.Roll(rbuf[k])
		s := hash2uint64(roll.Sum(hbuf))
		if *dostats {
			for _, m := range masks {
				if s&m == m {
					hits[m] += 1
				} else {
					break
				}
			}
		}
		k++
		n++
	}
	duration := time.Since(t)
	fmt.Printf("Rolled %s of data in %v (%s/s).\n",
		bytefmt.ByteSize(n),
		duration,
		bytefmt.ByteSize(n*1e9/uint64(duration)),
	)
	if *dostats {
		for i, m := range masks {
			frequency := "NaN"
			if hits[m] != 0 {
				frequency = bytefmt.ByteSize(n / hits[m])
			}
			fmt.Printf("%b (%d bits): %s\n", m, i+1, frequency)
		}
	}
}
