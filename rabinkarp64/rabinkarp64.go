package rabinkarp64

import "sync"

const Size = 8

type RabinKarp64 struct {
	value uint64

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	windowsize int
	window     []byte
	oldest     int
}

type tables struct {
	out [256]Pol
	mod [256]Pol
}

// cache precomputed tables, these are read-only anyway
var cache struct {
	entries map[Pol]tables
	sync.Mutex
}

func init() {
	cache.entries = make(map[Pol]tables)
}

func fillTables(p Pol, ws int) {
	cache.Lock()
	defer cache.Unlock()
	if _, ok := cache.entries[p]; ok {
		return
	}

	var t tables
	// calculate table for sliding out bytes. The byte to slide out is used as
	// the index for the table, the value contains the following:
	// out_table[b] = Hash(b || 0 ||        ...        || 0)
	//                          \ windowsize-1 zero bytes /
	// To slide out byte b_0 for window size w with known hash
	// H := H(b_0 || ... || b_w), it is sufficient to add out_table[b_0]:
	//    H(b_0 || ... || b_w) + H(b_0 || 0 || ... || 0)
	//  = H(b_0 + b_0 || b_1 + 0 || ... || b_w + 0)
	//  = H(    0     || b_1 || ...     || b_w)
	//
	// Afterwards a new byte can be shifted in.
	for b := 0; b < 256; b++ {
		var h Pol

		h = appendByte(h, byte(b), p)
		for i := 0; i < ws-1; i++ {
			h = appendByte(h, 0, p)
		}
		t.out[b] = h
	}

	// calculate table for reduction mod Polynomial
	k := p.Deg()
	for b := 0; b < 256; b++ {
		// mod_table[b] = A | B, where A = (b(x) * x^k mod pol) and  B = b(x) * x^k
		//
		// The 8 bits above deg(Polynomial) determine what happens next and so
		// these bits are used as a lookup to this table. The value is split in
		// two parts: Part A contains the result of the modulus operation, part
		// B is used to cancel out the 8 top bits so that one XOR operation is
		// enough to reduce modulo Polynomial
		t.mod[b] = Pol(uint64(b)<<uint(k)).Mod(p) | (Pol(b) << uint(k))
	}

	cache.entries[p] = t
}

func appendByte(hash Pol, b byte, pol Pol) Pol {
	hash <<= 8
	hash |= Pol(b)

	return hash.Mod(pol)
}

func New() *RabinKarp64 {
	return &RabinKarp64{}
}

func (d *RabinKarp64) Reset() {
	d.value = 0
}

// Size is 8 bytes
func (d *RabinKarp64) Size() int { return Size }

// BlockSize is 1 byte
func (d *RabinKarp64) BlockSize() int { return 1 }

// Write (re)initializes the rolling window with the input byte slice and
// adds its data to the digest. It never returns an error.
func (d *RabinKarp64) Write(data []byte) (int, error) {
	// Copy the window
	d.windowsize = len(data)
	if d.windowsize == 0 {
		d.windowsize = 1
	}
	if len(d.window) >= d.windowsize {
		d.window = d.window[:d.windowsize]
	} else {
		d.window = make([]byte, d.windowsize)
	}
	copy(d.window, data)

	// TODO: compute the checksum
	return len(d.window), nil
}

// Sum64 returns the hash as a uint64
func (d *RabinKarp64) Sum64() uint64 {
	return d.value
}

// Sum returns the hash as byte slice
func (d *RabinKarp64) Sum(b []byte) []byte {
	v := d.Sum64()
	return append(b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// Roll updates the checksum of the window from the entering byte. You
// MUST initialize a window with Write() before calling this method.
func (d *RabinKarp64) Roll(c byte) {
	// extract the entering/leaving bytes and update the circular buffer.
	//enter := uint32(c)
	//leave := uint32(d.window[d.oldest])
	d.window[d.oldest] = c
	d.oldest += 1
	if d.oldest >= d.windowsize {
		d.oldest = 0
	}

}
