/*
Package rollinghash implements rolling versions of some hashes
*/
package rollinghash

import (
	"hash"
	"io"
)

// DefaultWindowCap is the default capacity of the internal window of a
// new Hash.
const DefaultWindowCap = 64

// A Roller is a type that has the method Roll. Roll updates the hash of a
// rolling window from just the entering byte. You MUST call Write()
// BEFORE using this method and provide it with an initial window of size
// at least 1 byte. You can then call this method for every new byte
// entering the window. The byte leaving the window is automatically
// computed from a copy of the window internally kept in the checksum.
// This window is updated along with the internal state of the checksum
// every time Roll() is called.
type Roller interface {
	Roll(b byte)

	// WriteWindow writes the contents of the current window to w.
	//
	// It returns the number of bytes written and any error returned by
	// w.Write.
	WriteWindow(w io.Writer) (int, error)
}

// rollinghash.Hash extends hash.Hash by adding the method Roll. A
// rollinghash.Hash can be updated byte by byte, by specifying which byte
// enters the window.
// A rollinghash.Hash internally maintains a copy of the rolling window in
// order to keep track of the value of the byte exiting the window. This
// copy is updated with every call to Roll. The rolling window can be
// accessed through the io.Reader interface.
type Hash interface {
	hash.Hash
	Roller
}

// rollinghash.Hash32 extends hash.Hash by adding the method Roll. A
// rollinghash.Hash32 can be updated byte by byte, by specifying which
// byte enters the window.
// A rollinghash.Hash32 internally maintains a copy of the rolling window in
// order to keep track of the value of the byte exiting the window. This
// copy is updated with every call to Roll. The rolling window can be
// accessed through the io.Reader interface.
type Hash32 interface {
	hash.Hash32
	Roller
}

// rollinghash.Hash64 extends hash.Hash by adding the method Roll. A
// rollinghash.Hash64 can be updated byte by byte, by specifying which
// byte enters the window.
// A rollinghash.Hash64 internally maintains a copy of the rolling window in
// order to keep track of the value of the byte exiting the window. This
// copy is updated with every call to Roll. The rolling window can be
// accessed through the io.Reader interface.
type Hash64 interface {
	hash.Hash64
	Roller
}

// Chunker is the common interface satisfied by all CDC chunker implementations.
// It lets callers swap algorithms without changing iteration code. Sum returns
// the rolling fingerprint at the last content-defined boundary (AtMask true);
// it returns 0 on forced cuts.
type Chunker interface {
	Next() bool
	Bytes() []byte
	AtMask() bool
	Sum() uint64
	Err() error
	Reset(r io.Reader)
}

// batchRoller is an optional bulk fast path; see BatchRoll.
type batchRoller interface {
	BatchRoll(dst []uint64, data []byte, window int)
}

// boundaryRoller is an optional fused boundary fast path; see BatchBoundaries.
type boundaryRoller interface {
	BatchBoundaries(a, b []int32, data []byte, window int, mask uint64) (na, nb int)
}
