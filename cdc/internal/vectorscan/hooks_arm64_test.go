//go:build arm64 && !purego

package vectorscan

const (
	haveAVX2Path = false
	haveNEONPath = true
)

func getAVX2() bool        { return false }
func setAVX2(bool)         {}
func getAVX2Default() bool { return false }

func getNEON() bool        { return useNEON }
func setNEON(v bool)       { useNEON = v }
func getNEONDefault() bool { return true }
