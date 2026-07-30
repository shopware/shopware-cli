package deployment

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func writeTestFile(t *testing.T, root, name string) {
	t.Helper()

	path := filepath.Join(root, name)
	assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	assert.NoError(t, os.WriteFile(path, []byte(name), 0o644))
}

func archiveEntries(t *testing.T, data []byte) []string {
	t.Helper()

	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	assert.NoError(t, err)

	tarReader := tar.NewReader(gzReader)

	var entries []string
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		assert.NoError(t, err)

		entries = append(entries, header.Name)
	}

	return entries
}

func TestWriteProjectArchive(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, root, "composer.json")
	writeTestFile(t, root, "bin/console")
	writeTestFile(t, root, "public/index.php")
	writeTestFile(t, root, ".git/HEAD")
	writeTestFile(t, root, "node_modules/foo/index.js")
	writeTestFile(t, root, "var/cache/prod/app.php")
	writeTestFile(t, root, "public/media/image.png")
	writeTestFile(t, root, ".env")
	writeTestFile(t, root, ".shopware-project.local.yml")

	var buf bytes.Buffer
	err := writeProjectArchive(&buf, root, []string{"public/media", ".env"})
	assert.NoError(t, err)

	entries := archiveEntries(t, buf.Bytes())

	assert.Contains(t, entries, "composer.json")
	assert.Contains(t, entries, "bin/console")
	assert.Contains(t, entries, "public/index.php")

	// default excludes
	assert.NotContains(t, entries, ".git/HEAD")
	assert.NotContains(t, entries, ".git/")
	assert.NotContains(t, entries, "node_modules/foo/index.js")
	assert.NotContains(t, entries, "var/cache/prod/app.php")
	assert.NotContains(t, entries, ".shopware-project.local.yml")

	// configured excludes (shared paths)
	assert.NotContains(t, entries, "public/media/image.png")
	assert.NotContains(t, entries, ".env")
}

func TestWriteProjectArchivePreservesSymlinks(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, root, "vendor/bin/real-binary")
	assert.NoError(t, os.Symlink("real-binary", filepath.Join(root, "vendor", "bin", "linked")))

	var buf bytes.Buffer
	assert.NoError(t, writeProjectArchive(&buf, root, nil))

	gzReader, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
	assert.NoError(t, err)

	tarReader := tar.NewReader(gzReader)

	found := false
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		assert.NoError(t, err)

		if header.Name == "vendor/bin/linked" {
			found = true
			assert.Equal(t, byte(tar.TypeSymlink), header.Typeflag)
			assert.Equal(t, "real-binary", header.Linkname)
		}
	}

	assert.True(t, found, "symlink missing in archive")
}
