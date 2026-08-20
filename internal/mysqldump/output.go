package mysqldump

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
)

// Compression selects how the dump output is compressed.
type Compression string

const (
	CompressionNone Compression = ""
	CompressionGzip Compression = "gzip"
	CompressionZstd Compression = "zstd"
)

// ParseCompression validates a user-provided compression name.
func ParseCompression(s string) (Compression, error) {
	switch c := Compression(s); c {
	case CompressionNone, CompressionGzip, CompressionZstd:
		return c, nil
	default:
		return CompressionNone, fmt.Errorf("unsupported compression %q (supported: %s, %s)", s, CompressionGzip, CompressionZstd)
	}
}

// suffix returns the file extension appended to compressed dump files.
func (c Compression) suffix() string {
	switch c {
	case CompressionGzip:
		return ".gz"
	case CompressionZstd:
		return ".zst"
	case CompressionNone:
		return ""
	default:
		return ""
	}
}

// output layers an optional compressor over the dump destination. Close
// closes the compressor first so buffered data is flushed into the file
// before the file itself is closed. Closing again is a no-op, so callers can
// pair a deferred Close for error paths with an explicit, checked Close.
type output struct {
	io.Writer
	closers []io.Closer
}

func (o *output) Close() error {
	var errs []error
	for _, closer := range o.closers {
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	o.closers = nil

	return errors.Join(errs...)
}

// OpenOutput opens the dump destination. A path of "-" writes to stdout,
// which is never closed. With compression enabled the returned path carries
// the compression suffix. The returned writer must be closed to flush the
// compressor and the file.
func OpenOutput(path string, compression Compression) (io.WriteCloser, string, error) {
	var w io.Writer
	var closers []io.Closer

	if path == "-" {
		w = os.Stdout
	} else {
		path += compression.suffix()

		file, err := os.Create(path)
		if err != nil {
			return nil, "", err
		}

		w = file
		closers = []io.Closer{file}
	}

	switch compression {
	case CompressionGzip:
		gzipWriter := gzip.NewWriter(w)
		w = gzipWriter
		closers = append([]io.Closer{gzipWriter}, closers...)
	case CompressionZstd:
		zstdWriter, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
		if err != nil {
			for _, closer := range closers {
				_ = closer.Close()
			}
			return nil, "", err
		}
		w = zstdWriter
		closers = append([]io.Closer{zstdWriter}, closers...)
	case CompressionNone:
	}

	return &output{Writer: w, closers: closers}, path, nil
}
