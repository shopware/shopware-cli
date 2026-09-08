package project

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

// writeMinimalPlugin creates a minimal platform plugin so the upload command reaches the environment-resolution step.
func writeMinimalPlugin(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), testhelper.ComposerJSON{
		Name:        "frosh/frosh-test",
		Type:        "shopware-platform-plugin",
		License:     "MIT",
		Version:     "1.0.0",
		Require:     map[string]string{"shopware/core": "~6.6.0"},
		PluginClass: `FroshTest\FroshTest`,
		Label:       map[string]string{"de-DE": "Test", "en-GB": "Test"},
		Psr4:        map[string]string{`FroshTest\`: "src/"},
	}.String())
	testhelper.WriteFile(t, filepath.Join(dir, ".shopware-extension.yml"), `build:
  zip:
    composer:
      enabled: false
    assets:
      enabled: false
`)
	testhelper.WriteFile(t, filepath.Join(dir, "src", "FroshTest.php"), `<?php
namespace FroshTest;
use Shopware\Core\Framework\Plugin;
class FroshTest extends Plugin {}
`)

	return dir
}

func clearShopClientEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SHOPWARE_CLI_API_URL", "")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "")
	t.Setenv("SHOPWARE_CLI_API_USERNAME", "")
	t.Setenv("SHOPWARE_CLI_API_PASSWORD", "")
}

func setupEnvironmentConfig(t *testing.T) {
	t.Helper()
	clearShopClientEnv(t)
	t.Setenv("PROJECT_ROOT", t.TempDir())

	configPath := filepath.Join(t.TempDir(), ".shopware-project.yml")
	testhelper.WriteFile(t, configPath, `
url: http://127.0.0.1:9
compatibility_date: "2026-01-01"
admin_api:
  client_id: base-id
  client_secret: base-secret
environments:
  staging:
    url: http://127.0.0.1:29
    admin_api:
      client_id: staging-id
      client_secret: staging-secret
`)

	previousConfigPath := projectConfigPath
	previousEnvironmentName := environmentName
	projectConfigPath = configPath
	t.Cleanup(func() {
		projectConfigPath = previousConfigPath
		environmentName = previousEnvironmentName
	})
}

func TestNoEnvFlagPrefersLocalEnvironmentOverTopLevel(t *testing.T) {
	clearShopClientEnv(t)
	t.Setenv("PROJECT_ROOT", t.TempDir())

	configPath := filepath.Join(t.TempDir(), ".shopware-project.yml")
	testhelper.WriteFile(t, configPath, `
url: http://127.0.0.1:9
compatibility_date: "2026-01-01"
admin_api:
  client_id: base-id
  client_secret: base-secret
environments:
  local:
    url: http://127.0.0.1:7
    admin_api:
      client_id: local-id
      client_secret: local-secret
`)

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
	assert.Contains(t, err.Error(), "127.0.0.1:7", "without -e environments.local must be used")
	assert.NotContains(t, err.Error(), "127.0.0.1:9", "the deprecated top-level url must not override environments.local")
}

func TestNoEnvFlagUsesLocalWhenNoTopLevel(t *testing.T) {
	clearShopClientEnv(t)
	t.Setenv("PROJECT_ROOT", t.TempDir())

	configPath := filepath.Join(t.TempDir(), ".shopware-project.yml")
	testhelper.WriteFile(t, configPath, `
compatibility_date: "2026-01-01"
environments:
  local:
    url: http://127.0.0.1:7
    admin_api:
      client_id: local-id
      client_secret: local-secret
`)

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
	assert.Contains(t, err.Error(), "127.0.0.1:7", "without -e and no top-level shop, environments.local must be used")
}

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
