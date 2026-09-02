package chunkcore_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"

	"github.com/chmduquesne/rollinghash/v4/cdc/internal/chunkcore"
)

// fixedCut cuts every `size` bytes, forcing the max path.
type fixedCut struct{ size, max int }

func (f fixedCut) MaxSize() int { return f.max }
func (f fixedCut) Window() int  { return 8 }
func (f fixedCut) Cut(avail []byte, eof bool) (int, bool, uint64) {
	if len(avail) <= f.size {
		return len(avail), false, 0
	}
	return f.size, true, 42
}

// WindowDigest sums the window bytes so forced-cut Sum is a checkable function
// of exactly the last Window() bytes.
func (f fixedCut) WindowDigest(b []byte) uint64 {
	var s uint64
	for _, v := range b {
		s += uint64(v)
	}
	return s
}

func drain(t *testing.T, c *chunkcore.Core) ([][]byte, []int) {
	t.Helper()
	var chunks [][]byte
	var offs []int
	for c.Next() {
		chunks = append(chunks, append([]byte(nil), c.Bytes()...))
		offs = append(offs, c.Offset())
	}
	if err := c.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	return chunks, offs
}

func TestCoreChunksAndOffsets(t *testing.T) {
	data := bytes.Repeat([]byte("abcdefgh"), 5000) // 40000 bytes
	for _, size := range []int{1, 7, 4096, 39999, 40000, 40001} {
		c := chunkcore.New(bytes.NewReader(data), fixedCut{size: size, max: 8192}, nil)
		chunks, offs := drain(t, c)

		if joined := bytes.Join(chunks, nil); !bytes.Equal(joined, data) {
			t.Fatalf("size=%d: reassembled %d bytes, want %d", size, len(joined), len(data))
		}
		at := 0
		for i, ch := range chunks {
			if offs[i] != at {
				t.Fatalf("size=%d chunk %d: offset %d, want %d", size, i, offs[i], at)
			}
			at += len(ch)
		}
	}
}

func TestCoreOneByteReader(t *testing.T) {
	data := bytes.Repeat([]byte{1, 2, 3}, 9000)
	want, _ := drain(t, chunkcore.New(bytes.NewReader(data), fixedCut{size: 1000, max: 4096}, nil))
	got, _ := drain(t, chunkcore.New(iotest.OneByteReader(bytes.NewReader(data)), fixedCut{size: 1000, max: 4096}, nil))
	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("chunk %d differs", i)
		}
	}
}

func TestCoreReaderError(t *testing.T) {
	boom := errors.New("boom")
	c := chunkcore.New(iotest.ErrReader(boom), fixedCut{size: 10, max: 64}, nil)
	if c.Next() {
		t.Fatal("expected Next to return false on reader error")
	}
	if !errors.Is(c.Err(), boom) {
		t.Fatalf("Err = %v, want boom", c.Err())
	}
}

func TestCoreEmptyAndReset(t *testing.T) {
	c := chunkcore.New(bytes.NewReader(nil), fixedCut{size: 10, max: 64}, nil)
	if c.Next() {
		t.Fatal("empty: expected no chunks")
	}

	data := []byte("hello world, this is a test payload")
	c.Reset(bytes.NewReader(data))
	chunks, _ := drain(t, c)
	if joined := bytes.Join(chunks, nil); !bytes.Equal(joined, data) {
		t.Fatalf("after Reset: reassembled %q", joined)
	}
}

// growReader hands back large blocks so Core must grow its buffer past the
// initial max+readBlock capacity within a single fill sequence.
type bigBlockReader struct {
	data []byte
	pos  int
}

func (r *bigBlockReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// TestCoreSumWindow checks that Sum at a forced/final cut is WindowDigest of the
// last Window() bytes ending at the cut - including when compaction has run and
// that window straddles the previous chunk's end - and 0 only for a final chunk
// shorter than the window.
func TestCoreSumWindow(t *testing.T) {
	sumLast8 := func(b []byte) uint64 {
		var s uint64
		for _, v := range b[len(b)-8:] {
			s += uint64(v)
		}
		return s
	}

	// Big stream, small max: the buffer compacts several times before the tiny
	// 3-byte final chunk, whose 8-byte window reaches back past c.start into
	// the previous chunk.
	data := randData(600000 + 3)
	c := chunkcore.New(&bigBlockReader{data: data}, fixedCut{size: 20000, max: 32 * 1024}, nil)
	end, forced := 0, 0
	for c.Next() {
		end += len(c.Bytes())
		if c.ContentDefined() {
			continue
		}
		forced++
		if len(c.Bytes()) != 3 {
			t.Fatalf("forced chunk %d bytes, want the 3-byte tail", len(c.Bytes()))
		}
		if c.Sum() != sumLast8(data[end-8:end]) {
			t.Fatalf("straddling final window: Sum %d, want %d", c.Sum(), sumLast8(data[end-8:end]))
		}
	}
	if err := c.Err(); err != nil {
		t.Fatal(err)
	}
	if forced != 1 {
		t.Fatalf("saw %d forced cuts, want 1", forced)
	}

	// A whole stream shorter than the window: Sum stays 0.
	c = chunkcore.New(bytes.NewReader([]byte("abc")), fixedCut{size: 20000, max: 32 * 1024}, nil)
	if !c.Next() {
		t.Fatal("expected one chunk")
	}
	if c.Sum() != 0 {
		t.Fatalf("sub-window stream: Sum %d, want 0", c.Sum())
	}
}

// randData fills a slice with xorshift64 pseudo-random bytes.
func randData(n int) []byte {
	b := make([]byte, n)
	var x uint64 = 0x1234567
	for i := range b {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		b[i] = byte(x)
	}
	return b
}

func TestCoreBufferGrowth(t *testing.T) {
	data := bytes.Repeat([]byte{0xAB}, 500*1024)
	c := chunkcore.New(&bigBlockReader{data: data}, fixedCut{size: 200 * 1024, max: 256 * 1024}, nil)
	chunks, _ := drain(t, c)
	if joined := bytes.Join(chunks, nil); !bytes.Equal(joined, data) {
		t.Fatalf("reassembled %d bytes, want %d", len(joined), len(data))
	}
}
