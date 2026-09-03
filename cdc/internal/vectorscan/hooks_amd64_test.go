//go:build amd64 && !purego

package vectorscan

const haveAVX2Path = true

var avx2Default = detectAVX2()

func getAVX2() bool        { return useAVX2 }
func setAVX2(v bool)       { useAVX2 = v }
func getAVX2Default() bool { return avx2Default }
