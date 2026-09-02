package boxo

import (
	"io"
	"strings"
	"sync"
)

// SplitterFunc creates a Splitter from a reader and a specification string such
// as "mychunker-param1-param2". Register it with Register to make it available
// through FromString.
type SplitterFunc func(r io.Reader, chunker string) (Splitter, error)

var (
	splittersMu sync.RWMutex
	splitters   = map[string]SplitterFunc{}
)

func init() {
	Register("size", parseSizeString)
	Register("rabin", parseRabinString)
	Register("buzhash", parseBuzhashString)
}

// Register makes a custom chunker available to FromString under the given name,
// matched against the portion of the chunker string before the first dash. It
// panics if name is empty, contains a dash, fn is nil, or the name is already
// registered. It is safe for concurrent use.
//
// This registry is independent of github.com/ipfs/boxo/chunker's.
func Register(name string, fn SplitterFunc) {
	splittersMu.Lock()
	defer splittersMu.Unlock()
	if name == "" {
		panic("boxo: Register name is empty")
	}
	if strings.Contains(name, "-") {
		panic("boxo: Register name must not contain a dash: " + name)
	}
	if fn == nil {
		panic("boxo: Register fn is nil")
	}
	if _, dup := splitters[name]; dup {
		panic("boxo: Register called twice for chunker " + name)
	}
	splitters[name] = fn
}
