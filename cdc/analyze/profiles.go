package main

// A profile is a chunk-size target. min and max are always derived as
// target/32 and target*8, so the only thing that varies between profiles is the
// target itself — every algorithm sees the same 1 : 32 : 256 size envelope,
// just scaled.
type profile struct {
	name     string
	target   int
	override *sizeOpts // set only for a -min/-max custom profile
}

func (p profile) sizes() sizeOpts {
	if p.override != nil {
		return *p.override
	}
	return sizesFor(p.target)
}

// standardProfiles are modelled on what real systems ship (see README):
// 64 KiB ≈ casync/desync, 256 KiB ≈ Google Stadia / buildbarn, 1 MiB ≈ restic
// and plakar.
func standardProfiles() []profile {
	return []profile{
		{name: "sync", target: 64 << 10},
		{name: "artifact", target: 256 << 10},
		{name: "backup", target: 1 << 20},
	}
}

// sizeOpts holds the concrete min / normal / max a chunker is configured with.
type sizeOpts struct{ min, normal, max int }

// sizesFor derives the min/max envelope from a target: min = target/32,
// max = target*8.
func sizesFor(target int) sizeOpts {
	return sizeOpts{min: max(64, target>>5), normal: target, max: target << 3}
}
