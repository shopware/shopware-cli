package system

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"

	"github.com/klauspost/compress/zstd"
)

var (
	gzipMagic = []byte{0x1f, 0x8b}
	zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}
)

// DecompressReader detects gzip and zstd compression from the leading magic
// bytes and returns a transparently decompressing reader. Close the returned
// reader when it implements io.Closer to release decompressor resources.
func DecompressReader(r io.Reader) (io.Reader, error) {
	buffered := bufio.NewReaderSize(r, 1<<16)

	magic, err := buffered.Peek(4)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}

	switch {
	case bytes.HasPrefix(magic, gzipMagic):
		return gzip.NewReader(buffered)
	case bytes.HasPrefix(magic, zstdMagic):
		reader, err := zstd.NewReader(buffered)
		if err != nil {
			return nil, err
		}

		return reader.IOReadCloser(), nil
	}

	return buffered, nil
}
