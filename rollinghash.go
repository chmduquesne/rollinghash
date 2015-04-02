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
		for i := n; i < len(s); i++ {

			err := hash.Roll(s[i])
			if err != nil {
				log.Fatal(err)
			}

			sum := hash.Sum32()

			fmt.Printf("The adler32sum of %v is %x\n", s[i+1-n : i+1], sum)
		}
	}
*/
package rollinghash

import "hash"

// RollingHash is the common interface implemented by all rolling
// checksums. A RollingHash can be updated byte by byte, by specifying
// which byte enters the window.
type Hash interface {
	hash.Hash

	// Roll updates the hash of a rolling window from the entering byte.
	// A copy of the window is internally kept from the last Write().
	// Roll updates this copy and the internal state of the checksum, and
	// ideally (at least this is true for adler32), determines the new
	// hash just from the current state, the entering byte, and the
	// leaving byte.
	Roll(b byte) error
}

type Hash32 interface {
	hash.Hash32

	// Roll updates the hash of a rolling window from the entering byte.
	// A copy of the window is internally kept from the last Write().
	// Roll updates this copy and the internal state of the checksum, and
	// ideally (at least this is true for adler32), determines the new
	// hash just from the current state, the entering byte, and the
	// leaving byte.
	Roll(b byte) error
}

type Hash64 interface {
	hash.Hash64

	// Roll updates the hash of a rolling window from the entering byte.
	// A copy of the window is internally kept from the last Write().
	// Roll updates this copy and the internal state of the checksum, and
	// ideally (at least this is true for adler32), determines the new
	// hash just from the current state, the entering byte, and the
	// leaving byte.
	Roll(b byte) error
}
