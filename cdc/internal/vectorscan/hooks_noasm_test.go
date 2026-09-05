//go:build (!amd64 && !arm64) || purego

package vectorscan

const (
	haveAVX2Path = false
	haveNEONPath = false
)

func getAVX2() bool        { return false }
func setAVX2(bool)         {}
func getAVX2Default() bool { return false }

func getNEON() bool        { return false }
func setNEON(bool)         {}
func getNEONDefault() bool { return false }
