package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeAdminWatchFile(t *testing.T, projectRoot string, relPath, content string) {
	t.Helper()
	path := filepath.Join(projectRoot, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// writeAdminWebpackConfig marks the project as one whose Administration app
// still ships the webpack toolchain (Shopware 6.6).
func writeAdminWebpackConfig(t *testing.T, projectRoot string) {
	t.Helper()
	writeAdminWatchFile(t, projectRoot, "vendor/shopware/administration/Resources/app/administration/webpack.config.js", "module.exports = {}")
}

func TestAdminDevServerPort(t *testing.T) {
	t.Run("6.7 without webpack config serves vite", func(t *testing.T) {
		dir := t.TempDir()
		writeAdminWatchFile(t, dir, "vendor/shopware/administration/Resources/app/administration/vite.config.mts", "export default {}")
		assert.Equal(t, AdminVitePort, AdminDevServerPort(dir))
	})

	t.Run("6.6 defaults to webpack even with a vite config present", func(t *testing.T) {
		// 6.6 ships both configs; the dev script picks by feature flag and
		// falls back to webpack when var/config_js_features.json is missing.
		dir := t.TempDir()
		writeAdminWebpackConfig(t, dir)
		writeAdminWatchFile(t, dir, "vendor/shopware/administration/Resources/app/administration/vite.config.mts", "export default {}")
		assert.Equal(t, AdminWebpackPort, AdminDevServerPort(dir))
	})

	t.Run("6.6 with ADMIN_VITE disabled serves webpack", func(t *testing.T) {
		dir := t.TempDir()
		writeAdminWebpackConfig(t, dir)
		writeAdminWatchFile(t, dir, "var/config_js_features.json", `{"ADMIN_VITE": false, "admin.vite": false}`)
		assert.Equal(t, AdminWebpackPort, AdminDevServerPort(dir))
	})

	t.Run("6.6 with ADMIN_VITE enabled serves vite", func(t *testing.T) {
		dir := t.TempDir()
		writeAdminWebpackConfig(t, dir)
		writeAdminWatchFile(t, dir, "var/config_js_features.json", `{"ADMIN_VITE": true}`)
		assert.Equal(t, AdminVitePort, AdminDevServerPort(dir))
	})

	t.Run("broken feature dump falls back to webpack", func(t *testing.T) {
		dir := t.TempDir()
		writeAdminWebpackConfig(t, dir)
		writeAdminWatchFile(t, dir, "var/config_js_features.json", "not-json")
		assert.Equal(t, AdminWebpackPort, AdminDevServerPort(dir))
	})

	t.Run("missing platform assumes current tooling", func(t *testing.T) {
		assert.Equal(t, AdminVitePort, AdminDevServerPort(t.TempDir()))
	})
}
