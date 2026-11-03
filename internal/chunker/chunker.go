// Package chunker implements content-defined chunking (CDC) using a
// FastCDC-style gear rolling hash with normalized chunking.
//
// Why CDC instead of fixed-size blocks? If one byte is inserted at the start
// of a file, fixed-size blocking shifts every later block boundary, so every
// block hashes differently and deduplication collapses. CDC picks boundaries
// based on the *content* (when the rolling hash matches a mask), so boundaries
// re-synchronise right after the edit and only the chunks near the edit change.
package chunker

import (
	"errors"
	"io"
)

const (
	MinSize = 2 << 10  // 2 KiB  - never cut before this
	AvgSize = 8 << 10  // 8 KiB  - target average
	MaxSize = 64 << 10 // 64 KiB - always cut at this

	// Normalized chunking (FastCDC paper, section 3.3): use a *harder* mask
	// (more bits) before the average size and an *easier* mask after it.
	// This tightens the chunk-size distribution around AvgSize.
	maskS uint64 = ((1 << 15) - 1) << 49 // 15 bits: cut probability 1/32768
	maskL uint64 = ((1 << 11) - 1) << 53 // 11 bits: cut probability 1/2048
)

// gear is a table of 256 pseudo-random 64-bit values. It must be
// deterministic: two backups of the same data must produce identical
// boundaries, otherwise deduplication breaks across runs.
var gear [256]uint64

func init() {
	s := uint64(0x9E3779B97F4A7C15)
	for i := range gear {
		// xorshift64
		s ^= s << 13
		s ^= s >> 7
		s ^= s << 17
		gear[i] = s
	}
}

// Chunker splits a stream into content-defined chunks.
type Chunker struct {
	r          io.Reader
	buf        []byte
	start, end int
	eof        bool
}

// New returns a Chunker reading from r.
func New(r io.Reader) *Chunker {
	return &Chunker{r: r, buf: make([]byte, 4*MaxSize)}
}

// Next returns the next chunk. The returned slice is only valid until the
// next call to Next; callers that keep it must copy it. Returns io.EOF when
// the stream is exhausted.
func (c *Chunker) Next() ([]byte, error) {
	if c.end-c.start < MaxSize && !c.eof {
		if err := c.fill(); err != nil {
			return nil, err
		}
	}
	if c.start == c.end {
		return nil, io.EOF
	}
	n := Cut(c.buf[c.start:c.end])
	chunk := c.buf[c.start : c.start+n]
	c.start += n
	return chunk, nil
}

func (c *Chunker) fill() error {
	// Compact unread bytes to the front of the buffer.
	copy(c.buf, c.buf[c.start:c.end])
	c.end -= c.start
	c.start = 0
	for c.end < len(c.buf) {
		n, err := c.r.Read(c.buf[c.end:])
		c.end += n
		if errors.Is(err, io.EOF) {
			c.eof = true
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Cut returns the length of the first chunk in data.
func Cut(data []byte) int {
	n := len(data)
	if n <= MinSize {
		return n
	}
	if n > MaxSize {
		n = MaxSize
	}
	normal := AvgSize
	if n < normal {
		normal = n
	}
	var fp uint64
	i := MinSize
	for ; i < normal; i++ {
		fp = (fp << 1) + gear[data[i]]
		if fp&maskS == 0 {
			return i + 1
		}
	}
	for ; i < n; i++ {
		fp = (fp << 1) + gear[data[i]]
		if fp&maskL == 0 {
			return i + 1
		}
	}
	return n
}
