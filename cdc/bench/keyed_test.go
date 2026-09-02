package bench

import (
	"bytes"
	"encoding/binary"
	"testing"

	chunkers "github.com/PlakarKorp/go-cdc-chunkers"
	_ "github.com/PlakarKorp/go-cdc-chunkers/chunkers/fastcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/fastcdc"
	"github.com/chmduquesne/rollinghash/v4/cdc/internal/plakar"
	"github.com/chmduquesne/rollinghash/v4/gearhash64"
	"github.com/zeebo/blake3"
)

// plakarKeyedGearTable is the reference recipe for a keyed Gear table whose
// boundaries match PlakarKorp/go-cdc-chunkers' "kfastcdc" (its getGearTable):
// feed a keyed BLAKE3 hasher (32-byte key) the little-endian bytes of the base
// table, then read its XOF digest back as 256 little-endian uint64s. Pass the
// result to gearhash64.NewFromUint64Array and pair it with
// fastcdc.WithMasks(fastcdc.LegacyMaskS, fastcdc.LegacyMaskL).
//
// rollinghash ships no key-derivation helper of its own (it would pull a
// non-stdlib hash into the default build); callers who want keyed chunking copy
// this ~10 lines, or derive a table any other way.
func plakarKeyedGearTable(key []byte) ([256]uint64, error) {
	var table [256]uint64
	h, err := blake3.NewKeyed(key)
	if err != nil {
		return table, err
	}
	var buf [8]byte
	for _, v := range plakar.GearTable {
		binary.LittleEndian.PutUint64(buf[:], v)
		h.Write(buf[:])
	}
	dgst := make([]byte, 8*256)
	if _, err := h.Digest().Read(dgst); err != nil {
		return table, err
	}
	for i := range table {
		table[i] = binary.LittleEndian.Uint64(dgst[i*8:])
	}
	return table, nil
}

// TestKeyedFastCDCParityWithRealPlakar checks that cdc/fastcdc, fed a table from
// plakarKeyedGearTable and plakar's legacy masks, produces byte-identical
// boundaries to real plakar's "kfastcdc" (legacy keyed FastCDC) for several
// keys, configs and data shapes.
func TestKeyedFastCDCParityWithRealPlakar(t *testing.T) {
	keys := map[string][]byte{
		"zero":  make([]byte, 32),
		"ones":  bytes.Repeat([]byte{0x01}, 32),
		"mixed": []byte("0123456789abcdef0123456789abcdef"),
	}
	configs := map[string]opts{
		"default": {2 * 1024, 8 * 1024, 64 * 1024},
		"small":   {512, 4 * 1024, 32 * 1024},
		"tiny":    {128, 1024, 8 * 1024},
	}
	datasets := map[string][]byte{
		"rand512k":   randData(512 * 1024),
		"struct256k": structData(256 * 1024),
		"rand2000":   randData(2000),
		"tail":       randData(70*1024 + 123),
	}

	for kname, key := range keys {
		table, err := plakarKeyedGearTable(key)
		if err != nil {
			t.Fatalf("%s: plakarKeyedGearTable: %v", kname, err)
		}
		for cname, cfg := range configs {
			for dname, data := range datasets {
				po := plakarOpts(cfg)
				po.Key = key
				pc, err := chunkers.NewChunker("kfastcdc", bytes.NewReader(data), po)
				if err != nil {
					t.Fatalf("%s/%s: plakar NewChunker: %v", kname, cname, err)
				}
				want := drainPlakar(t, pc)

				oc := fastcdc.New(bytes.NewReader(data), gearhash64.NewFromUint64Array(table),
					cfg.min, cfg.normal, cfg.max,
					fastcdc.WithMasks(fastcdc.LegacyMaskS, fastcdc.LegacyMaskL))
				var got [][]byte
				for oc.Next() {
					got = append(got, append([]byte(nil), oc.Bytes()...))
				}
				if err := oc.Err(); err != nil {
					t.Fatalf("%s/%s/%s: fastcdc Err: %v", kname, cname, dname, err)
				}

				if len(got) != len(want) {
					t.Fatalf("%s/%s/%s: chunk count %d, want %d", kname, cname, dname, len(got), len(want))
				}
				for i := range want {
					if !bytes.Equal(got[i], want[i]) {
						t.Fatalf("%s/%s/%s: chunk %d differs (len %d vs %d)",
							kname, cname, dname, i, len(got[i]), len(want[i]))
					}
				}
			}
		}
	}
}
