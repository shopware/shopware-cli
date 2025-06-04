package shop

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigMerging(t *testing.T) {
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

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

	baseFilePath := path.Join(tmpDir, "base.yml")
	stagingFilePath := path.Join(tmpDir, "staging.yml")

	assert.NoError(t, os.WriteFile(baseFilePath, baseConfig, 0644))
	assert.NoError(t, os.WriteFile(stagingFilePath, stagingConfig, 0644))

	config, err := ReadConfig(stagingFilePath, false)
	assert.NoError(t, err)

	assert.NotNil(t, config.Sync)
	assert.NotNil(t, config.Sync.Config)
	assert.Len(t, config.Sync.Config, 1)
	assert.Equal(t, "xyz.nuonic.dev", config.Sync.Config[0].Settings["core.store.licenseHost"])
}
