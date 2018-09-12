/*

Package rollinghash implements rolling versions of some hashes

*/
package rollinghash

import "hash"

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

type RollingWindow struct {
	Bytes  []byte
	oldest int
}

func (r *RollingWindow) Reset() {
	r.Bytes = r.Bytes[:0]
	r.oldest = 0
}

func (r *RollingWindow) ReIndex() {
	if r.oldest != 0 {
		tmp := make([]byte, r.oldest)
		copy(tmp, r.Bytes[:r.oldest])
		copy(r.Bytes, r.Bytes[r.oldest:])
		copy(r.Bytes[len(r.Bytes)-r.oldest:], tmp)
		r.oldest = 0
	}
}

func (r *RollingWindow) Write(data []byte) {
	r.ReIndex()
	r.Bytes = append(r.Bytes, data...)
}

func (r *RollingWindow) Roll(enter byte) (leave byte) {
	leave = r.Bytes[r.oldest]
	r.Bytes[r.oldest] = enter
	r.oldest += 1
	if r.oldest >= len(r.Bytes) {
		r.oldest = 0
	}
	return
}

func NewRollingWindow() *RollingWindow {
	return &RollingWindow{
		Bytes:  make([]byte, 0, DefaultWindowCap),
		oldest: 0,
	}
}
