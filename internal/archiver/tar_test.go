package archiver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteTarGz(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "empty"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "bin", "console"), []byte("console"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "excluded"), []byte("private"), 0o600))
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Symlink("bin/console", filepath.Join(root, "console")))
	}

	var out bytes.Buffer
	require.NoError(t, WriteTarGz(t.Context(), &out, root, func(name string) bool { return name == "excluded" }))
	gz, err := gzip.NewReader(&out)
	require.NoError(t, err)
	defer func() { assert.NoError(t, gz.Close()) }()
	tr := tar.NewReader(gz)
	headers := map[string]*tar.Header{}
	contents := map[string]string{}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		headers[header.Name] = header
		data, err := io.ReadAll(tr)
		require.NoError(t, err)
		contents[header.Name] = string(data)
		assert.Zero(t, header.Uid)
		assert.Zero(t, header.Gid)
	}
	assert.Equal(t, "console", contents["bin/console"])
	assert.Contains(t, headers, "empty")
	assert.NotContains(t, headers, "excluded")
	assert.NotContains(t, headers, filepath.Base(root))
	if runtime.GOOS != "windows" {
		assert.EqualValues(t, 0o755, headers["bin/console"].Mode)
		require.Contains(t, headers, "console")
		assert.Equal(t, byte(tar.TypeSymlink), headers["console"].Typeflag)
		assert.Equal(t, "bin/console", headers["console"].Linkname)
	}
	// Consume the gzip footer to verify checksum/truncation, too.
	_, err = io.Copy(io.Discard, gz)
	require.NoError(t, err)
}

func TestContainedSymlinkRejectsUnsafeTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges on Windows")
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "outside"), []byte("private"), 0o600))
	for _, target := range []string{"../outside", filepath.Join(parent, "outside"), "missing"} {
		link := filepath.Join(root, "link")
		require.NoError(t, os.Symlink(target, link))
		var out bytes.Buffer
		require.Error(t, WriteTarGz(t.Context(), &out, root, nil))
		require.NoError(t, os.Remove(link))
	}
}

type failingTarWriter struct{ err error }

func (w failingTarWriter) Write([]byte) (int, error) { return 0, w.err }

func TestWriteTarGzReturnsCloseErrors(t *testing.T) {
	writeErr := errors.New("output failed")
	// An empty archive can fail while flushing the tar/gzip footer.
	require.ErrorIs(t, WriteTarGz(t.Context(), failingTarWriter{writeErr}, t.TempDir(), nil), writeErr)
}

func TestWriteTarGzCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var out bytes.Buffer
	require.ErrorIs(t, WriteTarGz(ctx, &out, t.TempDir(), nil), context.Canceled)
	_, err := ContextReader(ctx, bytes.NewBufferString("data")).Read(make([]byte, 4))
	require.ErrorIs(t, err, context.Canceled)
}
