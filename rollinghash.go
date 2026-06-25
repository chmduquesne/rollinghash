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

// BulkRoller is an optional fast path. A Hash that implements it computes
// the rolling checksum of every window-sized slice of data in one call,
// owning the loop so it can index the leaving byte directly and keep
// several accumulators in registers. Callers normally reach it indirectly
// through helpers, exactly as io.Copy reaches io.ReaderFrom.
//
// BulkRoll writes the rolling hash at every position into dst, which must
// have len(data)-window+1 elements. It is semantically equivalent to
// Write(data[:window]) followed by a Roll for each subsequent byte,
// recording Sum64 after each step. It does not modify the receiver's state.
type BulkRoller interface {
	BulkRoll(dst []uint64, data []byte, window int)
}

// BoundaryRoller is an optional fast path for content-defined chunking. A Hash
// that implements it scans data for the window positions where the rolling
// checksum satisfies sum & mask == 0, without materializing the full checksum
// stream — the per-position test is fused into the hashing loop. Callers
// normally reach it indirectly through Chunker.
//
// BulkBoundaries records the matching positions of the two interleaved lanes
// separately: it appends the ascending lane-A indices to a[:na] and the
// ascending lane-B indices to b[:nb], where every index in a precedes every
// index in b, so the caller reads a[:na] followed by b[:nb] as a single
// ascending run. a and b must each have len >= len(data)-window+1. Position i
// denotes the window data[i:i+window]. It does not modify the receiver.
type BoundaryRoller interface {
	BulkBoundaries(a, b []int32, data []byte, window int, mask uint64) (na, nb int)
}
