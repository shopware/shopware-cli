package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceMapCleanup(t *testing.T) {
	t.Run("invalid directory", func(t *testing.T) {
		assert.NoError(t, cleanupJavaScriptSourceMaps("invalid-directory"))
	})

	t.Run("does not touch js", func(t *testing.T) {
		tmpDir := t.TempDir()

		assert.NoError(t, cleanupJavaScriptSourceMaps(tmpDir))

		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "random.js"), []byte("test"), 0o644))

		assert.NoError(t, cleanupJavaScriptSourceMaps(tmpDir))

		assert.FileExists(t, filepath.Join(tmpDir, "random.js"))
	})

	t.Run("removes map files", func(t *testing.T) {
		tmpDir := t.TempDir()

		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foo.js.map"), []byte("test"), 0o644))

		assert.NoError(t, cleanupJavaScriptSourceMaps(tmpDir))

		assert.NoFileExists(t, filepath.Join(tmpDir, "foo.js.map"))
	})

	t.Run("remove sourcemap comments", func(t *testing.T) {
		tmpDir := t.TempDir()

		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.js"), []byte("console.log//# sourceMappingURL=test.js.map"), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.js.map"), []byte("test"), 0o644))

		assert.NoError(t, cleanupJavaScriptSourceMaps(tmpDir))

		content, err := os.ReadFile(filepath.Join(tmpDir, "test.js"))
		assert.NoError(t, err)

		assert.Equal(t, "console.log", string(content))
	})
}
