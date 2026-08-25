package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorOnFixtureProject(t *testing.T) {
	oldConfigPath := projectConfigPath
	projectConfigPath = ""
	t.Cleanup(func() { projectConfigPath = oldConfigPath })

	newDoctorCmd := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())
		return cmd
	}

	t.Run("bare project", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"shopware/core":"~6.6.0"}}`), 0o644))

		out, err := captureStdout(func() error {
			return projectDoctor.RunE(newDoctorCmd(), []string{dir})
		})
		require.NoError(t, err)
		assert.Contains(t, out, "Shopware version:")
		assert.Contains(t, out, "not found, using fallback")
		assert.Contains(t, out, "No extensions or bundles detected")
	})

	t.Run("project with a plugin", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"shopware/core":"~6.6.0"}}`), 0o644))
		pluginDir := filepath.Join(dir, "custom", "plugins", "FroshTest")
		require.NoError(t, os.MkdirAll(pluginDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "composer.json"), []byte(`{
			"name": "frosh/frosh-test",
			"type": "shopware-platform-plugin",
			"version": "1.0.0",
			"require": { "shopware/core": "~6.6.0" },
			"extra": { "shopware-plugin-class": "FroshTest\\FroshTest" }
		}`), 0o644))

		out, err := captureStdout(func() error {
			return projectDoctor.RunE(newDoctorCmd(), []string{dir})
		})
		require.NoError(t, err)
		assert.Contains(t, out, "FroshTest")
		assert.Contains(t, out, filepath.Join("custom", "plugins", "FroshTest"))
	})
}
