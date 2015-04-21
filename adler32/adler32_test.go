package adler32_test

import (
	rollsum "github.com/chmduquesne/rollinghash/adler32"
	"hash/adler32"
	"testing"
)

const data = "The quick brown fox jumps over the lazy dog"

func TestRolling(t *testing.T) {
	s := []byte(data)

	vanilla := adler32.New()
	rolling := rollsum.New()

	// arbitrary window len
	n := 16

	// Load the window into the rolling hash
	rolling.Write(s[:n])

	// Roll it and compare the result with full re-calculus every time
	for i := n; i < len(s); i++ {

		vanilla.Reset()
		vanilla.Write(s[i-n+1 : i+1])

		err := rolling.Roll(s[i])
		if err != nil {
			t.Fatal(err)
		}

		if vanilla.Sum32() != rolling.Sum32() {
			t.Fatalf("%v: expected %x, got %x",
				s[i-n+1:i+1], vanilla.Sum32(), rolling.Sum32())
		}

	}
}

func TestUninitialized(t *testing.T) {
	s := []byte(data)
	hash := rollsum.New()
	err := hash.Roll(s[0])

	if err == nil {
		t.Fatal("Rolling with an uninitialized window should trigger an error")
	}
}

func BenchmarkRolling(b *testing.B) {
	window := make([]byte, 1024)
	for i := range window {
		window[i] = byte(i)
	}

	r := rollsum.New()
	b.ResetTimer()

	r.Write(window)
	for i := 0; i < b.N; i++ {
		r.Roll(byte(1024 + i))
		r.Sum32()
	}
}
