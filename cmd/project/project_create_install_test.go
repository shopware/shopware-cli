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

func TestPrintCreateSummaryOmitsMakeSetup(t *testing.T) {
	t.Run("docker interactive success screen", func(t *testing.T) {
		output, err := captureStdout(func() error {
			printCreateSummary(t.Context(), &createOptions{
				projectFolder: "demo-shop",
				useDocker:     true,
				interactive:   true,
			})
			return nil
		})
		require.NoError(t, err)

		assert.Contains(t, output, "Access your shop")
		assert.NotContains(t, output, "make setup")
		assert.Contains(t, output, "shopware-cli project dev")
		assert.Contains(t, output, "http://127.0.0.1:8000")
		assert.Contains(t, output, "http://127.0.0.1:8000/admin")
	})

	t.Run("non-interactive does not print access heading", func(t *testing.T) {
		output, err := captureStdout(func() error {
			printCreateSummary(t.Context(), &createOptions{
				projectFolder: "demo-shop",
				useDocker:     true,
			})
			return nil
		})
		require.NoError(t, err)

		assert.NotContains(t, output, "Access your shop")
		assert.NotContains(t, output, "make setup")
	})
}
