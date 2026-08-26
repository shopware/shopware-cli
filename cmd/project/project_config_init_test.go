package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestProjectEnvFlagHelpDefaultsToEnvironmentsLocal(t *testing.T) {
	assert.Contains(t, environmentFlagUsage, "environments.local")
	assert.Contains(t, environmentFlagUsage, "deprecated")
}

func TestSetLocalShopFromConfigInitShape(t *testing.T) {
	cfg := &shop.Config{}
	cfg.SetLocalShop("http://localhost", &shop.ConfigAdminApi{
		Username: "admin",
		Password: "secret",
	})

	assert.Empty(t, cfg.URL)
	assert.Nil(t, cfg.AdminApi)
	require.NotNil(t, cfg.Environments["local"])
	assert.Equal(t, "http://localhost", cfg.Environments["local"].URL)
	assert.Equal(t, "admin", cfg.Environments["local"].AdminApi.Username)
}
