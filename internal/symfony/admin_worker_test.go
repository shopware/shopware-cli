package symfony

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeAdminWorkerConfig(t *testing.T, body string) string {
	t.Helper()

	root := t.TempDir()
	packagesDir := filepath.Join(root, "config", "packages")
	require.NoError(t, os.MkdirAll(packagesDir, 0o755))

	if body != "" {
		require.NoError(t, os.WriteFile(filepath.Join(packagesDir, "shopware.yaml"), []byte(body), 0o644))
	}

	return root
}

func TestIsAdminWorkerEnabled(t *testing.T) {
	t.Run("enabled by default when not configured", func(t *testing.T) {
		root := writeAdminWorkerConfig(t, "")

		pc, err := NewProjectConfig(root)
		require.NoError(t, err)

		enabled, err := pc.IsAdminWorkerEnabled("dev")
		require.NoError(t, err)
		assert.True(t, enabled)
	})

	t.Run("disabled when set to false", func(t *testing.T) {
		root := writeAdminWorkerConfig(t, "shopware:\n  admin_worker:\n    enable_admin_worker: false\n")

		pc, err := NewProjectConfig(root)
		require.NoError(t, err)

		enabled, err := pc.IsAdminWorkerEnabled("dev")
		require.NoError(t, err)
		assert.False(t, enabled)
	})

	t.Run("enabled when set to true", func(t *testing.T) {
		root := writeAdminWorkerConfig(t, "shopware:\n  admin_worker:\n    enable_admin_worker: true\n")

		pc, err := NewProjectConfig(root)
		require.NoError(t, err)

		enabled, err := pc.IsAdminWorkerEnabled("dev")
		require.NoError(t, err)
		assert.True(t, enabled)
	})

	t.Run("respects when@dev override", func(t *testing.T) {
		root := writeAdminWorkerConfig(t, "shopware:\n  admin_worker:\n    enable_admin_worker: true\n\nwhen@dev:\n  shopware:\n    admin_worker:\n      enable_admin_worker: false\n")

		pc, err := NewProjectConfig(root)
		require.NoError(t, err)

		devEnabled, err := pc.IsAdminWorkerEnabled("dev")
		require.NoError(t, err)
		assert.False(t, devEnabled)

		prodEnabled, err := pc.IsAdminWorkerEnabled("prod")
		require.NoError(t, err)
		assert.True(t, prodEnabled)
	})

	t.Run("enabled when project has no config/packages", func(t *testing.T) {
		pc, err := NewProjectConfig(t.TempDir())
		require.NoError(t, err)

		enabled, err := pc.IsAdminWorkerEnabled("dev")
		require.NoError(t, err)
		assert.True(t, enabled)
	})
}
