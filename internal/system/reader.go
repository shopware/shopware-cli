package system

import "io"

// CountingReader wraps a reader and counts the bytes read through it, e.g.
// for progress reporting against a known total size.
type CountingReader struct {
	Reader io.Reader

	count int64
}

func (c *CountingReader) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	c.count += int64(n)

	return n, err
}

// BytesRead returns the number of bytes read so far.
func (c *CountingReader) BytesRead() int64 {
	return c.count
}
