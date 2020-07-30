// Package window provides common code for dealing with sliding windows.
package window

import "io"

// MoveLeft re-arranges window so that the oldest element is at index 0.
func MoveLeft(window []byte, oldest int) {
	// This is the old swap-by-reverse trick (see Programming Pearls):
	// R(a)+R(b) = R(b+a), so b+a = R(R(a)+R(b)).
	reverse(window[:oldest])
	reverse(window[oldest:])
	reverse(window)
}

func reverse(w []byte) {
	for i := 0; i < len(w)/2; i++ {
		j := len(w) - i - 1
		w[i], w[j] = w[j], w[i]
	}
}

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
