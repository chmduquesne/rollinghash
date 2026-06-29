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

// BatchRoller walks an io.Reader and yields, per batch, the rolling checksum
// at every window position together with the bytes those checksums cover.
// The underlying hash must implement BatchRoll.
//
//	s := NewBatchRoller(r, h, window)
//	for s.Next() {
//		sums, data := s.Sums(), s.Bytes()
//		for i, sum := range sums {
//			// sum == rolling checksum of data[i:i+window]
//		}
//	}
//	if err := s.Err(); err != nil { ... }
//
// Per-batch guarantee: len(Sums()) == len(Bytes())-window+1, and Sums()[i]
// is the checksum of Bytes()[i:i+window]. Consecutive batches overlap by
// window-1 bytes so no window position is skipped or duplicated.
//
// Sums and Bytes are valid only until the next call to Next.
// Use WithBuffer to control the batch size (default 64 KiB).
// Reset reuses internal allocations across streams.
type BatchRoller interface {
	Next() bool
	Bytes() []byte
	Sums() []uint64
	Err() error
	Reset(r io.Reader)
}

// Chunker splits an io.Reader into content-defined chunks. The underlying
// hash must implement BatchBoundaries.
//
//	c := NewChunker(r, h, window, mask)
//	for c.Next() {
//		chunk := c.Bytes()
//		if c.ContentDefined() {
//			// content-defined boundary; Sum() is the hit value
//		} else {
//			// forced cut at max, or the final chunk
//		}
//	}
//	if err := c.Err(); err != nil { ... }
//
// Bytes is valid only until the next call to Next. ContentDefined reports whether the
// current chunk ended at a mask hit (true) or was forced at max / end of
// stream (false). Sum returns the rolling checksum at a mask boundary; it
// returns 0 on forced cuts. Use WithBoundaries to set minimum and maximum chunk
// sizes (defaults: 0 and math.MaxInt). Reset reuses internal allocations across streams.
type Chunker interface {
	Next() bool
	Bytes() []byte
	ContentDefined() bool
	Sum() uint64
	Err() error
	Reset(r io.Reader)
}

// hashBatchRoller is the interface a Hash must implement to be usable with
// NewBatchRoller. BatchRoll must write len(data)-window+1 checksums to dst,
// where dst[i] is the rolling checksum of data[i:i+window].
type hashBatchRoller interface {
	BatchRoll(dst []uint64, data []byte, window int)
}

// hashBoundaryRoller is the interface a Hash must implement to be usable with
// NewChunker. BatchBoundaries must report all window positions in data where
// the rolling checksum satisfies sum&mask==0, writing them into a and b
// (two independent lanes for ILP); it returns the counts na and nb.
type hashBoundaryRoller interface {
	BatchBoundaries(a, b []int32, data []byte, window int, mask uint64) (na, nb int)
}
