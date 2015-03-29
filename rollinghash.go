/*
Package rollinghash implements rolling versions of some hashes

Example:

	package main

	import (
		"fmt"
		"github.com/chmduquesne/rollinghash/adler32"
		"log"
	)

	func main() {
		s := []byte("The brown fox jumps over the lazy dog")
		hash := adler32.New()

		// window len
		n := 16

		// Load the window
		hash.Write(s[0:n])

		// Roll it
		for i := 1; i < len(s)-n; i++ {

			err := hash.Roll(s[i-1], s[i+n-1])
			if err != nil {
				log.Fatal(err)
			}

			sum := hash.Sum32()

			fmt.Printf("%v has checksum %x\n", s[i:i+n], sum)
		}
	}
*/
package rollinghash

import "hash"

// RollingHash is the common interface implemented by all rolling
// checksums. A RollingHash can be updated byte by byte, by specifying
// which byte enters the window, and which byte leaves it.
type RollingHash interface {
	hash.Hash

	// Roll updates the hash of a rolling window from the leaving byte and
	// the entering byte. It assumes the size of the window from the
	// first Write(), and if Nothing was ever written using Write(), it
	// triggers an error. Several calls to Write() will modify the
	// assumed window size every time.
	Roll(oldbyte, newbyte byte) error
}

type RollingHash32 interface {
	RollingHash
	Sum32() uint32
}

type RollingHash64 interface {
	RollingHash
	Sum64() uint64
}
