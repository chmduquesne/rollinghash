// Adapted from restic

package rabinkarp64

import "sync"

const Size = 8

type tables struct {
	out [256]Pol
	mod [256]Pol
}

// tables are cacheable for a given pol and windowsize
type index struct {
	pol        Pol
	windowsize int
}

type RabinKarp64 struct {
	pol      Pol
	tables   tables
	polShift uint
	value    uint64

	// window is treated like a circular buffer, where the oldest element
	// is indicated by d.oldest
	window []byte
	oldest int
}

// cache precomputed tables, these are read-only anyway
var cache struct {
	// For a given polynom and a given window size, we get a table
	entries map[index]tables
	sync.Mutex
}

func init() {
	cache.entries = make(map[index]tables)
}

func (d *RabinKarp64) initTables() {
	windowsize := len(d.window)
	idx := index{d.pol, windowsize}
	cache.Lock()
	defer cache.Unlock()
	if t, ok := cache.entries[idx]; ok {
		d.tables = t
		return
	}

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

		h <<= 8
		h |= Pol(b)
		h = h.Mod(d.pol)
		for i := 0; i < windowsize-1; i++ {
			h <<= 8
			h |= Pol(0)
			h = h.Mod(d.pol)
		}
		d.tables.out[b] = h
	}

	// calculate table for reduction mod Polynomial
	k := d.pol.Deg()
	for b := 0; b < 256; b++ {
		// mod_table[b] = A | B, where A = (b(x) * x^k mod pol) and  B = b(x) * x^k
		//
		// The 8 bits above deg(Polynomial) determine what happens next and so
		// these bits are used as a lookup to this table. The value is split in
		// two parts: Part A contains the result of the modulus operation, part
		// B is used to cancel out the 8 top bits so that one XOR operation is
		// enough to reduce modulo Polynomial
		d.tables.mod[b] = Pol(uint64(b)<<uint(k)).Mod(d.pol) | (Pol(b) << uint(k))
	}

	cache.entries[idx] = d.tables
}

func NewFromPol(p Pol) *RabinKarp64 {
	res := &RabinKarp64{
		pol: p,
	}
	res.Reset()
	return res
}

func New() *RabinKarp64 {
	p, err := RandomPolynomial(1)
	if err != nil {
		panic(err)
	}
	return NewFromPol(p)
}

func (d *RabinKarp64) Reset() {
	d.value = 0
	d.polShift = uint(d.pol.Deg() - 8)
}

// Size is 8 bytes
func (d *RabinKarp64) Size() int { return Size }

// BlockSize is 1 byte
func (d *RabinKarp64) BlockSize() int { return 1 }

// Write (re)initializes the rolling window with the input byte slice and
// adds its data to the digest. It never returns an error.
func (d *RabinKarp64) Write(data []byte) (int, error) {
	// Copy the window
	l := len(data)
	if l == 0 {
		l = 1
	}
	if len(d.window) >= l {
		d.window = d.window[:l]
	} else {
		d.window = make([]byte, l)
	}
	copy(d.window, data)

	for _, b := range d.window {
		d.value <<= 8
		d.value |= uint64(b)
		d.value = uint64(Pol(d.value).Mod(d.pol))
	}

	d.initTables()

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
	enter := c
	leave := uint64(d.window[d.oldest])
	d.window[d.oldest] = c
	d.oldest += 1
	if d.oldest >= len(d.window) {
		d.oldest = 0
	}

	d.value ^= uint64(d.tables.out[leave])
	index := byte(d.value >> d.polShift)
	d.value <<= 8
	d.value |= uint64(enter)
	d.value ^= uint64(d.tables.mod[index])
}
