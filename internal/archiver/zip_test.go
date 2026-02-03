package archiver

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateZip(t *testing.T) {
	t.Run("creates valid zip file", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create source directory with files
		sourceDir := filepath.Join(tmpDir, "source")
		require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "subdir"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "file1.txt"), []byte("content1"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "subdir", "file2.txt"), []byte("content2"), 0644))

		zipPath := filepath.Join(tmpDir, "test.zip")
		err := CreateZip(sourceDir, zipPath)
		require.NoError(t, err)

		// Verify zip file exists
		assert.FileExists(t, zipPath)

		// Verify zip content
		zipReader, err := zip.OpenReader(zipPath)
		require.NoError(t, err)
		defer zipReader.Close()

		fileNames := make([]string, 0)
		for _, f := range zipReader.File {
			fileNames = append(fileNames, f.Name)
		}

		assert.Contains(t, fileNames, "file1.txt")
		assert.Contains(t, fileNames, filepath.Join("subdir", "file2.txt"))
	})

	t.Run("handles empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		sourceDir := filepath.Join(tmpDir, "empty")
		require.NoError(t, os.MkdirAll(sourceDir, 0755))

		zipPath := filepath.Join(tmpDir, "empty.zip")
		err := CreateZip(sourceDir, zipPath)
		require.NoError(t, err)

		assert.FileExists(t, zipPath)
	})
}

func TestUnzip(t *testing.T) {
	t.Run("extracts files correctly", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a zip file in memory
		buf := new(bytes.Buffer)
		zipWriter := zip.NewWriter(buf)

		// Add files to zip
		files := map[string]string{
			"file1.txt":        "content1",
			"subdir/file2.txt": "content2",
		}

		for name, content := range files {
			w, err := zipWriter.Create(name)
			require.NoError(t, err)
			_, err = w.Write([]byte(content))
			require.NoError(t, err)
		}
		require.NoError(t, zipWriter.Close())

		// Create zip reader
		zipReader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.NoError(t, err)

		// Extract
		destDir := filepath.Join(tmpDir, "extracted")
		require.NoError(t, os.MkdirAll(destDir, 0755))
		err = Unzip(zipReader, destDir)
		require.NoError(t, err)

		// Verify extracted files
		content1, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
		require.NoError(t, err)
		assert.Equal(t, "content1", string(content1))

		content2, err := os.ReadFile(filepath.Join(destDir, "subdir", "file2.txt"))
		require.NoError(t, err)
		assert.Equal(t, "content2", string(content2))
	})

	t.Run("rejects zip slip attack", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a malicious zip with path traversal
		buf := new(bytes.Buffer)
		zipWriter := zip.NewWriter(buf)

		// Try to write outside the destination directory
		w, err := zipWriter.Create("../../../etc/passwd")
		require.NoError(t, err)
		_, err = w.Write([]byte("malicious content"))
		require.NoError(t, err)
		require.NoError(t, zipWriter.Close())

		zipReader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.NoError(t, err)

		destDir := filepath.Join(tmpDir, "extracted")
		require.NoError(t, os.MkdirAll(destDir, 0755))

		err = Unzip(zipReader, destDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "illegal file path")
	})
}

func TestAddZipFiles(t *testing.T) {
	t.Run("adds files recursively", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create source structure
		sourceDir := filepath.Join(tmpDir, "source")
		require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "a", "b"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "root.txt"), []byte("root"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "a", "level1.txt"), []byte("level1"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "a", "b", "level2.txt"), []byte("level2"), 0644))

		// Create zip
		buf := new(bytes.Buffer)
		zipWriter := zip.NewWriter(buf)

		err := AddZipFiles(zipWriter, sourceDir, "prefix")
		require.NoError(t, err)
		require.NoError(t, zipWriter.Close())

		// Verify
		zipReader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.NoError(t, err)

		fileNames := make([]string, 0)
		for _, f := range zipReader.File {
			fileNames = append(fileNames, f.Name)
		}

		assert.Contains(t, fileNames, filepath.Join("prefix", "root.txt"))
		assert.Contains(t, fileNames, filepath.Join("prefix", "a", "level1.txt"))
		assert.Contains(t, fileNames, filepath.Join("prefix", "a", "b", "level2.txt"))
	})
}
