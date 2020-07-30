// Package window provides common code for dealing with sliding windows.
package window

import "io"

func Write(w io.Writer, window []byte, oldest int) (n int, err error) {
	// Copy the older bytes.
	if oldest < len(window) {
		n, err = w.Write(window[oldest:])
	}
	// Then the newer bytes.
	if err == nil && oldest > 0 {
		var n2 int
		n2, err = w.Write(window[:oldest])
		n += n2
	}
	return
}
