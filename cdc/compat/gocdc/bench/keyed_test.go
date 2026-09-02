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
// plakarKeyedGearTable, produces byte-identical boundaries to real plakar's
// keyed FastCDC for several keys, configs and data shapes. Both reachable keyed
// paths are covered: "kfastcdc" (always legacy masks) and "fastcdc-v1.0.0" with
// a Key set (derived masks, once the size triple leaves plakar's legacy-mask
// shortcut).
func TestKeyedFastCDCParityWithRealPlakar(t *testing.T) {
	keys := map[string][]byte{
		"zero":  make([]byte, 32),
		"ones":  bytes.Repeat([]byte{0x01}, 32),
		"mixed": []byte("0123456789abcdef0123456789abcdef"),
	}
	defaultCfg := opts{2 * 1024, 8 * 1024, 64 * 1024}
	configs := map[string]opts{
		"default": defaultCfg,
		"small":   {512, 4 * 1024, 32 * 1024},
		"tiny":    {128, 1024, 8 * 1024},
	}
	datasets := map[string][]byte{
		"rand512k":   randData(512 * 1024),
		"struct256k": structData(256 * 1024),
		"rand2000":   randData(2000),
		"tail":       randData(70*1024 + 123),
	}

	// variant pairs a plakar algorithm name with the mask option cdc/fastcdc
	// needs to match plakar's internal choice for that name and config.
	variants := []struct {
		name    string
		algo    string
		maskOpt func(opts) []fastcdc.Option
		skip    func(cname string, cfg opts) bool
	}{
		{
			name: "kfastcdc",
			algo: "kfastcdc",
			maskOpt: func(opts) []fastcdc.Option {
				return []fastcdc.Option{fastcdc.WithMasks(fastcdc.LegacyMaskS, fastcdc.LegacyMaskL)}
			},
		},
		{
			name:    "fastcdc-v1.0.0+key",
			algo:    "fastcdc-v1.0.0",
			maskOpt: func(opts) []fastcdc.Option { return nil }, // derived masks
			// plakar still forces legacy masks at the default size triple,
			// which the derived-mask path would not match; skip it here (the
			// kfastcdc variant already covers default sizes).
			skip: func(_ string, cfg opts) bool { return cfg == defaultCfg },
		},
	}

	for kname, key := range keys {
		table, err := plakarKeyedGearTable(key)
		if err != nil {
			t.Fatalf("%s: plakarKeyedGearTable: %v", kname, err)
		}
		for _, v := range variants {
			for cname, cfg := range configs {
				if v.skip != nil && v.skip(cname, cfg) {
					continue
				}
				for dname, data := range datasets {
					po := plakarOpts(cfg)
					po.Key = key
					pc, err := chunkers.NewChunker(v.algo, bytes.NewReader(data), po)
					if err != nil {
						t.Fatalf("%s/%s/%s: plakar NewChunker: %v", v.name, kname, cname, err)
					}
					want := drainPlakar(t, pc)

					oc := fastcdc.New(bytes.NewReader(data), gearhash64.NewFromUint64Array(table),
						cfg.min, cfg.normal, cfg.max, v.maskOpt(cfg)...)
					var got [][]byte
					for oc.Next() {
						got = append(got, append([]byte(nil), oc.Bytes()...))
					}
					if err := oc.Err(); err != nil {
						t.Fatalf("%s/%s/%s/%s: fastcdc Err: %v", v.name, kname, cname, dname, err)
					}

					if len(got) != len(want) {
						t.Fatalf("%s/%s/%s/%s: chunk count %d, want %d", v.name, kname, cname, dname, len(got), len(want))
					}
					for i := range want {
						if !bytes.Equal(got[i], want[i]) {
							t.Fatalf("%s/%s/%s/%s: chunk %d differs (len %d vs %d)",
								v.name, kname, cname, dname, i, len(got[i]), len(want[i]))
						}
					}
				}
			}
		}
	}
}
