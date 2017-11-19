/*

Package rollinghash implements rolling versions of some hashes

*/
package rollinghash

import "hash"

const DefaultWindowCap = 64

type Roller interface {
	// Roll updates the hash of a rolling window from just the entering
	// byte. You MUST call Write() and provide it with an initial window
	// of size at least 1 byte before using this method. You can then call
	// this method for every new byte entering the window. A copy of the
	// window is internally kept, and is updated along with the internal
	// state of the checksum every time Roll() is called.
	Roll(b byte)
}

// rollinghash.Hash extends hash.Hash by adding the method Roll. A
// rollinghash.Hash can be updated byte by byte, by specifying which byte
// enters the window.
type Hash interface {
	hash.Hash
	Roller
}

// rollinghash.Hash32 extends hash.Hash by adding the method Roll. A
// rollinghash.Hash32 can be updated byte by byte, by specifying which
// byte enters the window.
type Hash32 interface {
	hash.Hash32
	Roller
}

// rollinghash.Hash64 extends hash.Hash by adding the method Roll. A
// rollinghash.Hash64 can be updated byte by byte, by specifying which
// byte enters the window.
type Hash64 interface {
	hash.Hash64
	Roller
}
