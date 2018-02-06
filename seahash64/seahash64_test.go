package seahash64_test

import (
	"hash"
	"testing"

	"github.com/chmduquesne/rollinghash"
	rollsum "github.com/chmduquesne/rollinghash/seahash64"
)

// Prove that we implement rollinghash.Hash64
var _ = rollinghash.Hash64(rollsum.New())

// Prove that we implement hash.Hash64
var _ = hash.Hash64(rollsum.New())

func TestGolden(t *testing.T) {
	h := rollsum.New()
	h.Write([]byte("to be or not to be"))
	if h.Sum64() != 1988685042348123509 {
		t.Errorf("expected 1988685042348123509, got %v", h.Sum64())
	}
}
