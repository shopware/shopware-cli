package mysqldump

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCompression(t *testing.T) {
	cases := []struct {
		input    string
		expected Compression
	}{
		{"", CompressionNone},
		{"gzip", CompressionGzip},
		{"zstd", CompressionZstd},
	}

	for _, tc := range cases {
		compression, err := ParseCompression(tc.input)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, compression)
	}

	_, err := ParseCompression("lz4")
	assert.ErrorContains(t, err, `unsupported compression "lz4"`)
}

func TestOpenOutputPlainFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.sql")

	w, finalPath, err := OpenOutput(path, CompressionNone)
	require.NoError(t, err)
	assert.Equal(t, path, finalPath)

	_, err = io.WriteString(w, "SELECT 1;")
	require.NoError(t, err)
	require.NoError(t, w.Close())

	content, err := os.ReadFile(finalPath)
	require.NoError(t, err)
	assert.Equal(t, "SELECT 1;", string(content))
}

func TestOpenOutputGzip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.sql")

	w, finalPath, err := OpenOutput(path, CompressionGzip)
	require.NoError(t, err)
	assert.Equal(t, path+".gz", finalPath)

	_, err = io.WriteString(w, "SELECT 1;")
	require.NoError(t, err)
	require.NoError(t, w.Close())

	file, err := os.Open(finalPath)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	reader, err := gzip.NewReader(file)
	require.NoError(t, err)

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "SELECT 1;", string(content))
}

func TestOpenOutputZstd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.sql")

	w, finalPath, err := OpenOutput(path, CompressionZstd)
	require.NoError(t, err)
	assert.Equal(t, path+".zst", finalPath)

	_, err = io.WriteString(w, "SELECT 1;")
	require.NoError(t, err)
	require.NoError(t, w.Close())

	file, err := os.Open(finalPath)
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	reader, err := zstd.NewReader(file)
	require.NoError(t, err)
	defer reader.Close()

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "SELECT 1;", string(content))
}

func TestOpenOutputStdoutIsNotClosed(t *testing.T) {
	w, finalPath, err := OpenOutput("-", CompressionNone)
	require.NoError(t, err)
	assert.Equal(t, "-", finalPath)

	// Close must not close os.Stdout: writing afterwards still works.
	require.NoError(t, w.Close())
	_, err = os.Stdout.Stat()
	assert.NoError(t, err)
}
