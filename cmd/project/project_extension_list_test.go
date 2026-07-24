package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEnvironmentConfig(t *testing.T) {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), ".shopware-project.yml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
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
`), 0o644))

	previousConfigPath := projectConfigPath
	previousEnvironmentName := environmentName
	projectConfigPath = configPath
	t.Cleanup(func() {
		projectConfigPath = previousConfigPath
		environmentName = previousEnvironmentName
	})
}

func TestExtensionListTargetsSelectedEnvironment(t *testing.T) {
	setupEnvironmentConfig(t)

	environmentName = "staging"
	projectExtensionListCmd.SetContext(t.Context())

	err := projectExtensionListCmd.RunE(projectExtensionListCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "127.0.0.1:29", "command must dial the staging environment URL")
	assert.NotContains(t, err.Error(), "127.0.0.1:9/", "command must not dial the base URL")
}

func TestExtensionListRejectsUnknownEnvironment(t *testing.T) {
	setupEnvironmentConfig(t)

	environmentName = "nonexistent"
	projectExtensionListCmd.SetContext(t.Context())

	err := projectExtensionListCmd.RunE(projectExtensionListCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `environment "nonexistent" not found`)
}
