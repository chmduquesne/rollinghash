//go:build !amd64 || purego

package vectorscan

const haveAVX2Path = false

func getAVX2() bool        { return false }
func setAVX2(bool)         {}
func getAVX2Default() bool { return false }
