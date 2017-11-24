package rollinghash_test

import (
	"bytes"
	"testing"

	"github.com/chmduquesne/rollinghash"
	_adler32 "github.com/chmduquesne/rollinghash/adler32"
	"github.com/chmduquesne/rollinghash/buzhash32"
	"github.com/chmduquesne/rollinghash/buzhash64"
)

var hashes = []struct {
	classic rollinghash.Hash
	rolling rollinghash.Hash
}{
	{_adler32.New(), _adler32.New()},
	{buzhash32.New(), buzhash32.New()},
	{buzhash64.New(), buzhash64.New()},
}

func TestAll(t *testing.T) {
	s := []byte("The quick brown fox jumps over the lazy dog")

	for _, h := range hashes {
		classic := h.classic
		rolling := h.rolling

		// buffers to write the sums
		bufc := make([]byte, 0, 8)
		bufr := make([]byte, 0, 8)

		// Window len
		n := 16

		// Load the window into the rolling hash
		rolling.Write(s[:n])

		// Roll it and compare the result with full re-calculus every time
		for i := n; i < len(s); i++ {

			// Reset and write the window in classic
			classic.Reset()
			classic.Write(s[i-n+1 : i+1])

			// Roll the incoming byte in rolling
			rolling.Roll(s[i])

			// Compare the hashes
			hashc := classic.Sum(bufc)
			hashr := rolling.Sum(bufr)
			if !bytes.Equal(hashc, hashr) {
				t.Errorf("%v: expected %s, got %s",
					s[i-n+1:i+1], string(hashc), string(hashr))
			}
		}
	}

}
