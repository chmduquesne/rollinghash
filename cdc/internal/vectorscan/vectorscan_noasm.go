//go:build (!amd64 && !arm64) || purego

package vectorscan

func maxByte(d []byte) byte                 { return maxByteGeneric(d) }
func minByte(d []byte) byte                 { return minByteGeneric(d) }
func indexCmp(d []byte, t byte, op int) int { return indexCmpGeneric(d, t, op) }
