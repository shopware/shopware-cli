package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeAdminAppFile(t *testing.T, projectRoot, name string) {
	t.Helper()
	adminApp := filepath.Join(projectRoot, "vendor", "shopware", "administration", "Resources", "app", "administration")
	require.NoError(t, os.MkdirAll(adminApp, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(adminApp, name), []byte("{}"), 0o644))
}

func TestAdminDevServerPort(t *testing.T) {
	t.Run("vite config means 5173", func(t *testing.T) {
		dir := t.TempDir()
		writeAdminAppFile(t, dir, "vite.config.mts")
		assert.Equal(t, AdminVitePort, AdminDevServerPort(dir))
	})

	t.Run("webpack config means 8080", func(t *testing.T) {
		dir := t.TempDir()
		writeAdminAppFile(t, dir, "webpack.config.js")
		assert.Equal(t, AdminWebpackPort, AdminDevServerPort(dir))
	})

	t.Run("vite wins when both exist", func(t *testing.T) {
		dir := t.TempDir()
		writeAdminAppFile(t, dir, "vite.config.mts")
		writeAdminAppFile(t, dir, "webpack.config.js")
		assert.Equal(t, AdminVitePort, AdminDevServerPort(dir))
	})

	t.Run("missing platform assumes current tooling", func(t *testing.T) {
		assert.Equal(t, AdminVitePort, AdminDevServerPort(t.TempDir()))
	})
}
