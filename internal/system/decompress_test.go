package system

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecompressReaderPlain(t *testing.T) {
	reader, err := DecompressReader(strings.NewReader("SELECT 1;"))
	require.NoError(t, err)

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "SELECT 1;", string(content))
}

func TestDecompressReaderGzip(t *testing.T) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	_, err := writer.Write([]byte("SELECT 'gz';"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	reader, err := DecompressReader(&buf)
	require.NoError(t, err)

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "SELECT 'gz';", string(content))
}

func TestDecompressReaderZstd(t *testing.T) {
	var buf bytes.Buffer
	writer, err := zstd.NewWriter(&buf)
	require.NoError(t, err)
	_, err = writer.Write([]byte("SELECT 'zst';"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	reader, err := DecompressReader(&buf)
	require.NoError(t, err)

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "SELECT 'zst';", string(content))
}

func TestDecompressReaderEmpty(t *testing.T) {
	reader, err := DecompressReader(strings.NewReader(""))
	require.NoError(t, err)

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Empty(t, content)
}

func TestDecompressReaderShortPlainInput(t *testing.T) {
	reader, err := DecompressReader(strings.NewReader(";"))
	require.NoError(t, err)

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, ";", string(content))
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("disk error") }

func TestDecompressReaderPropagatesReadError(t *testing.T) {
	_, err := DecompressReader(failingReader{})
	assert.ErrorContains(t, err, "disk error")
}
