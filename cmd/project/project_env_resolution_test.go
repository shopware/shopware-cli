package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeMinimalPlugin creates a minimal platform plugin the upload command can
// load, so its RunE reaches the environment-resolution step.
func writeMinimalPlugin(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{
		"name": "frosh/frosh-test",
		"type": "shopware-platform-plugin",
		"license": "MIT",
		"version": "1.0.0",
		"require": { "shopware/core": "~6.6.0" },
		"autoload": { "psr-4": { "FroshTest\\": "src/" } },
		"extra": {
			"shopware-plugin-class": "FroshTest\\FroshTest",
			"label": { "de-DE": "Test", "en-GB": "Test" }
		}
	}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".shopware-extension.yml"), []byte(
		"build:\n  zip:\n    composer:\n      enabled: false\n    assets:\n      enabled: false\n",
	), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "FroshTest.php"),
		[]byte("<?php\nnamespace FroshTest;\nuse Shopware\\Core\\Framework\\Plugin;\nclass FroshTest extends Plugin {}\n"), 0o644))

	return dir
}

// TestReadConfigWithEnvironmentPropagatesConfigError ensures a config-read
// failure is surfaced rather than swallowed by the environment resolution.
func TestReadConfigWithEnvironmentPropagatesConfigError(t *testing.T) {
	previousConfigPath := projectConfigPath
	previousEnvironmentName := environmentName
	t.Cleanup(func() {
		projectConfigPath = previousConfigPath
		environmentName = previousEnvironmentName
	})

	projectConfigPath = filepath.Join(t.TempDir(), "does-not-exist.yml")
	environmentName = "staging"

	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())

	_, err := readConfigWithEnvironment(cmd, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot find project configuration file")
}

// TestNoEnvFlagKeepsBaseConfig ensures that without -e the base url and
// credentials are used even when an environments.local entry exists, so the
// fix does not silently retarget existing setups.
func TestNoEnvFlagKeepsBaseConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), ".shopware-project.yml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
url: http://127.0.0.1:9
compatibility_date: "2026-01-01"
admin_api:
  client_id: base-id
  client_secret: base-secret
environments:
  local:
    url: http://127.0.0.1:7
`), 0o644))

	previousConfigPath := projectConfigPath
	previousEnvironmentName := environmentName
	projectConfigPath = configPath
	environmentName = ""
	t.Cleanup(func() {
		projectConfigPath = previousConfigPath
		environmentName = previousEnvironmentName
	})

	projectExtensionListCmd.SetContext(t.Context())

	err := projectExtensionListCmd.RunE(projectExtensionListCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "127.0.0.1:9", "without -e the base URL must be used")
	assert.NotContains(t, err.Error(), "127.0.0.1:7", "environments.local must not silently retarget commands")
}

// TestAdminAPICommandsResolveEnvironment verifies every project command that
// talks to a shop honors the -e/--env flag: an unknown environment is rejected,
// and a known environment's URL is the one contacted.
func TestAdminAPICommandsResolveEnvironment(t *testing.T) {
	pluginDir := writeMinimalPlugin(t)

	cases := []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{"admin-api", projectAdminApiCmd, []string{"GET", "/_info/config"}},
		{"clear-cache", projectClearCacheCmd, nil},
		{"extension activate", projectExtensionActivateCmd, []string{"Foo"}},
		{"extension deactivate", projectExtensionDeactivateCmd, []string{"Foo"}},
		{"extension delete", projectExtensionDeleteCmd, []string{"Foo"}},
		{"extension install", projectExtensionInstallCmd, []string{"Foo"}},
		{"extension list", projectExtensionListCmd, nil},
		{"extension outdated", projectExtensionOutdatedCmd, nil},
		{"extension uninstall", projectExtensionUninstallCmd, []string{"Foo"}},
		{"extension update", projectExtensionUpdateCmd, []string{"Foo"}},
		{"extension upload", projectExtensionUploadCmd, []string{pluginDir}},
		{"upgrade-check", projectUpgradeCheckCmd, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/rejects unknown environment", func(t *testing.T) {
			setupEnvironmentConfig(t)
			environmentName = "nonexistent"
			tc.cmd.SetContext(t.Context())

			err := tc.cmd.RunE(tc.cmd, tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), `environment "nonexistent" not found`,
				"command must reject an unknown environment instead of silently using the base config")
		})

		t.Run(tc.name+"/targets selected environment", func(t *testing.T) {
			setupEnvironmentConfig(t)
			environmentName = "staging"
			tc.cmd.SetContext(t.Context())

			err := tc.cmd.RunE(tc.cmd, tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "127.0.0.1:29",
				"command must contact the staging environment URL")
			assert.NotContains(t, err.Error(), "127.0.0.1:9/",
				"command must not contact the base URL when -e staging is given")
		})
	}
}
