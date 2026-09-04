package main

// minDiv and maxMul set the min/max envelope around every profile's target:
// min = target/minDiv, max = target*maxMul.
const (
	minDiv = 2
	maxMul = 8
)

// A profile is a chunk-size target. min and max are derived from it, so the
// only thing that varies between profiles is the target itself — every
// algorithm sees the same size envelope, just scaled.
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

// sizesFor derives the min/max envelope from a target using minDiv / maxMul.
func sizesFor(target int) sizeOpts {
	return sizeOpts{min: max(64, target/minDiv), normal: target, max: target * maxMul}
}
