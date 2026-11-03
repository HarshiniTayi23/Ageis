package chunker

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"testing"
)

func split(t *testing.T, data []byte) [][]byte {
	t.Helper()
	var out [][]byte
	c := New(bytes.NewReader(data))
	for {
		ch, err := c.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, append([]byte(nil), ch...))
	}
}

func randomData(n int, seed int64) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(seed)).Read(b)
	return b
}

func TestReassemblesExactly(t *testing.T) {
	data := randomData(3<<20, 1)
	if got := bytes.Join(split(t, data), nil); !bytes.Equal(got, data) {
		t.Fatal("chunks do not reassemble to the original")
	}
}

func TestSizeBounds(t *testing.T) {
	chunks := split(t, randomData(4<<20, 2))
	var total int
	for i, c := range chunks {
		total += len(c)
		if len(c) > MaxSize {
			t.Fatalf("chunk %d larger than MaxSize: %d", i, len(c))
		}
		if len(c) < MinSize && i != len(chunks)-1 {
			t.Fatalf("non-final chunk %d smaller than MinSize: %d", i, len(c))
		}
	}
	avg := total / len(chunks)
	if avg < AvgSize/2 || avg > AvgSize*2 {
		t.Fatalf("average chunk size %d far from target %d", avg, AvgSize)
	}
	t.Logf("%d chunks, avg %d bytes", len(chunks), avg)
}

// The property that makes CDC worth it: an insertion near the start of a
// file must NOT change most of the chunks after it.
func TestBoundaryShiftResistance(t *testing.T) {
	orig := randomData(2<<20, 3)
	edited := append(append(append([]byte{}, orig[:1000]...), []byte("INSERTED BYTES")...), orig[1000:]...)

	seen := map[string]bool{}
	for _, c := range split(t, orig) {
		seen[string(c)] = true
	}
	after := split(t, edited)
	reused := 0
	for _, c := range after {
		if seen[string(c)] {
			reused++
		}
	}
	ratio := float64(reused) / float64(len(after))
	t.Logf("reused %d/%d chunks (%.1f%%) after a 14-byte insertion", reused, len(after), 100*ratio)
	if ratio < 0.95 {
		t.Fatalf("expected >=95%% chunk reuse, got %.1f%%", 100*ratio)
	}
}

func BenchmarkChunker(b *testing.B) {
	data := randomData(16<<20, 4)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := New(bytes.NewReader(data))
		for {
			if _, err := c.Next(); err != nil {
				break
			}
		}
	}
}
