package shop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigMerging(t *testing.T) {
	tmpDir := t.TempDir()

	t.Chdir(tmpDir)

	baseConfig := []byte(`
admin_api:
  client_id: ${SHOPWARE_CLI_CLIENT_ID}
  client_secret: ${SHOPWARE_CLI_CLIENT_SECRET}
dump:
  where:
    customer: "email LIKE '%@nuonic.de' OR email LIKE '%@xyz.com'"
  nodata:
    - promotion
`)

	stagingConfig := []byte(`
url: https://xyz.nuonic.dev
include:
  - base.yml
sync:
  config:
    - settings:
        core.store.licenseHost: xyz.nuonic.dev
`)

	baseFilePath := filepath.Join(tmpDir, "base.yml")
	stagingFilePath := filepath.Join(tmpDir, "staging.yml")

	assert.NoError(t, os.WriteFile(baseFilePath, baseConfig, 0644))
	assert.NoError(t, os.WriteFile(stagingFilePath, stagingConfig, 0644))

	config, err := ReadConfig(stagingFilePath, false)
	assert.NoError(t, err)

	assert.NotNil(t, config.Sync)
	assert.NotNil(t, config.Sync.Config)
	assert.Len(t, config.Sync.Config, 1)
	assert.Equal(t, "xyz.nuonic.dev", config.Sync.Config[0].Settings["core.store.licenseHost"])

	assert.NoError(t, os.RemoveAll(tmpDir))
}

func TestConfigNoScripts(t *testing.T) {
	tmpDir := t.TempDir()

	t.Chdir(tmpDir)

	configWithNoScripts := []byte(`
url: https://example.com
build:
  no_scripts: true
`)

	configPath := filepath.Join(tmpDir, "test.yml")
	assert.NoError(t, os.WriteFile(configPath, configWithNoScripts, 0644))

	config, err := ReadConfig(configPath, false)
	assert.NoError(t, err)

	assert.NotNil(t, config.Build)
	assert.True(t, config.Build.NoScripts)

	assert.NoError(t, os.RemoveAll(tmpDir))
}

func TestConfigNoScriptsDefault(t *testing.T) {
	tmpDir := t.TempDir()

	t.Chdir(tmpDir)

	configWithoutNoScripts := []byte(`
url: https://example.com
build:
  disable_asset_copy: false
`)

	configPath := filepath.Join(tmpDir, "test.yml")
	assert.NoError(t, os.WriteFile(configPath, configWithoutNoScripts, 0644))

	config, err := ReadConfig(configPath, false)
	assert.NoError(t, err)

	assert.NotNil(t, config.Build)
	assert.False(t, config.Build.NoScripts)

	assert.NoError(t, os.RemoveAll(tmpDir))
}
