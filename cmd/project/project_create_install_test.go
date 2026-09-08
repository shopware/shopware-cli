package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestNewProjectConfig(t *testing.T) {
	t.Run("enables OpenSearch indexing on install for Shopware PaaS", func(t *testing.T) {
		cfg := newProjectConfig(&createOptions{
			selectedDeployment: shop.DeploymentShopwarePaaS,
		}, "")

		require.NotNil(t, cfg.ConfigDeployment)
		require.NotNil(t, cfg.ConfigDeployment.OpenSearch)
		assert.True(t, cfg.ConfigDeployment.OpenSearch.IndexOnInstall)
	})

	t.Run("does not add deployment configuration for other deployment methods", func(t *testing.T) {
		cfg := newProjectConfig(&createOptions{
			selectedDeployment: shop.DeploymentContainer,
			useDocker:          true,
		}, "8.4")

		assert.Nil(t, cfg.ConfigDeployment)
		require.NotNil(t, cfg.Docker)
		assert.Equal(t, "8.4", cfg.Docker.PHP.Version)
	})
}
