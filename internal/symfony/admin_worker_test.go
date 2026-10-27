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

func TestAdminWorkerSetting(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		enabled    bool
		configured bool
	}{
		{name: "not configured is the enabled default", body: "", enabled: true, configured: false},
		{name: "explicitly disabled", body: "shopware:\n  admin_worker:\n    enable_admin_worker: false\n", enabled: false, configured: true},
		{name: "explicitly enabled", body: "shopware:\n  admin_worker:\n    enable_admin_worker: true\n", enabled: true, configured: true},
		{name: "non-boolean value counts as not configured", body: "shopware:\n  admin_worker:\n    enable_admin_worker: maybe\n", enabled: true, configured: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pc, err := NewProjectConfig(writeAdminWorkerConfig(t, tc.body))
			require.NoError(t, err)

			enabled, configured, err := pc.AdminWorkerSetting("dev")
			require.NoError(t, err)
			assert.Equal(t, tc.enabled, enabled)
			assert.Equal(t, tc.configured, configured)
		})
	}
}
