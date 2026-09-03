// Package gearscan holds the shared inner loops of the Gear-based
// content-defined chunkers: Scan4 finds the first position where the windowless
// accumulating Gear fingerprint clears a mask (cdc/fastcdc, cdc/jumpchunker);
// Max finds the position where it is largest (cdc/maxcdc, cdc/repmaxcdc). It is
// internal and not part of the public API.
package gearscan

// Scan4 scans data[lo:hi] for the first index i at which the running Gear
// fingerprint — seeded with fp and advanced one byte at a time as
// fp = (fp<<1) + g[b] — satisfies fp & mask == 0. It returns that index with
// hit=true and the fingerprint value there; if no position clears the mask it
// returns hi, false, and the fingerprint at hi.
//
// The scan runs four bytes per iteration. The recurrence is serial, but issuing
// the four data[]/g[] dependent loads as a group (instead of one per loop turn
// behind a branch) lets them pipeline, roughly doubling throughput on the
// load-latency-bound common path. The caller must ensure hi <= len(data).
func Scan4(g *[256]uint64, data []byte, lo, hi int, mask, fp uint64) (pos int, hit bool, endFP uint64) {
	data = data[:hi:hi] // fold the bound so data[i:i+4] / data[i] need no check
	i := lo
	for ; i+4 <= hi; i += 4 {
		b := data[i : i+4]
		g0, g1, g2, g3 := g[b[0]], g[b[1]], g[b[2]], g[b[3]]

		if fp = (fp << 1) + g0; fp&mask == 0 {
			return i, true, fp
		}
		if fp = (fp << 1) + g1; fp&mask == 0 {
			return i + 1, true, fp
		}
		if fp = (fp << 1) + g2; fp&mask == 0 {
			return i + 2, true, fp
		}
		if fp = (fp << 1) + g3; fp&mask == 0 {
			return i + 3, true, fp
		}
	}
	for ; i < hi; i++ {
		if fp = (fp << 1) + g[data[i]]; fp&mask == 0 {
			return i, true, fp
		}
	}
	return hi, false, fp
}

// Max advances the Gear fingerprint (seeded with fp) over data[lo:hi] as
// fp = (fp<<1) + g[b] and returns the offset past lo of the last position at
// which the fingerprint reached a new strict maximum (0 if nothing beat the
// seed) together with that maximal value. It is the boundary search of MaxCDC
// and RepMaxCDC — cut before the position with the highest fingerprint. The
// caller must ensure hi <= len(data).
//
// Like Scan4, the scan runs four bytes per iteration: the recurrence is serial
// but issuing the four g[data[]] loads as a straight-line group lets them
// pipeline ahead of the dependent fp updates (~40% faster than a scalar loop on
// random data; the new-maximum branch compiles to conditional moves). The scalar
// tail loop below is the spec.
func Max(g *[256]uint64, data []byte, lo, hi int, fp uint64) (bestOff int, bestFP uint64) {
	data = data[:hi:hi] // fold the bound so data[i:i+4] / data[i] need no check
	bestFP = fp
	i := lo
	for ; i+4 <= hi; i += 4 {
		b := data[i : i+4 : i+4]
		g0, g1, g2, g3 := g[b[0]], g[b[1]], g[b[2]], g[b[3]]
		base := i - lo + 1
		if fp = (fp << 1) + g0; bestFP < fp {
			bestFP, bestOff = fp, base
		}
		if fp = (fp << 1) + g1; bestFP < fp {
			bestFP, bestOff = fp, base+1
		}
		if fp = (fp << 1) + g2; bestFP < fp {
			bestFP, bestOff = fp, base+2
		}
		if fp = (fp << 1) + g3; bestFP < fp {
			bestFP, bestOff = fp, base+3
		}
	}
	for ; i < hi; i++ {
		if fp = (fp << 1) + g[data[i]]; bestFP < fp {
			bestFP, bestOff = fp, i-lo+1
		}
	}
	return
}

// Digest returns the windowless Gear fingerprint of b: fp seeded 0 and advanced
// as fp = (fp<<1) + g[c] for every byte. Since the shift discards bits past 63,
// for len(b) >= 64 this equals the fingerprint a Scan4 chain would hold after
// the same trailing 64 bytes; the cdc engine uses it to report Sum at a forced
// or final cut.
func Digest(g *[256]uint64, b []byte) uint64 {
	var fp uint64
	for _, c := range b {
		fp = (fp << 1) + g[c]
	}
	return fp
}
