package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The existing image-proxy tests exercise a replica of the handler stack;
// these cover the real RunE's fail-fast validations, which all return before
// any server starts.
func TestImageProxyRunEValidatesUpstreamAndPublicDir(t *testing.T) {
	oldURL, oldConfigPath := imageProxyURL, projectConfigPath
	t.Cleanup(func() {
		imageProxyURL = oldURL
		projectConfigPath = oldConfigPath
	})

	run := func(t *testing.T, root string) error {
		t.Helper()
		t.Setenv("PROJECT_ROOT", root)
		projectImageProxyCmd.SetContext(t.Context())
		return projectImageProxyCmd.RunE(projectImageProxyCmd, nil)
	}

	t.Run("missing upstream", func(t *testing.T) {
		root := t.TempDir()
		projectConfigPath = filepath.Join(root, ".shopware-project.yml")
		imageProxyURL = ""

		err := run(t, root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upstream URL must be provided")
	})

	t.Run("missing public dir", func(t *testing.T) {
		root := t.TempDir()
		projectConfigPath = filepath.Join(root, ".shopware-project.yml")
		imageProxyURL = "https://upstream.example.com"

		err := run(t, root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "public folder not found")
	})

	// The config fallback is consulted when the flag is absent: it passes the
	// upstream check and still fails at the public-dir check, proving order.
	t.Run("config fallback provides the upstream", func(t *testing.T) {
		root := t.TempDir()
		configPath := filepath.Join(root, ".shopware-project.yml")
		require.NoError(t, os.WriteFile(configPath,
			[]byte("compatibility_date: 2026-03-01\nimage_proxy:\n  url: https://upstream.example.com\n"), 0o644))
		projectConfigPath = configPath
		imageProxyURL = ""

		err := run(t, root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "public folder not found")
	})
}
