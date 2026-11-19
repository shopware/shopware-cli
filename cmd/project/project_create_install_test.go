package project

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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

		plain := ansi.Strip(output)
		assert.Contains(t, plain, "Access your shop")
		assert.NotContains(t, plain, "make setup")
		assert.Contains(t, plain, "shopware-cli project dev")
		assert.Contains(t, plain, "http://127.0.0.1:8000")
		assert.Contains(t, plain, "http://127.0.0.1:8000/admin")
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

		plain := ansi.Strip(output)
		assert.NotContains(t, plain, "Access your shop")
		assert.NotContains(t, plain, "make setup")
		assert.NotContains(t, plain, "shopware-cli project validate")
	})
}

func TestPrintCreateSummaryWorkflowGuideposts(t *testing.T) {
	for _, ci := range []string{shop.CINone, shop.CIGitHub, shop.CIGitLab} {
		for _, docker := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/docker=%t", ci, docker), func(t *testing.T) {
				output, err := captureStdout(func() error {
					printCreateSummary(t.Context(), &createOptions{
						projectFolder: ".",
						selectedCI:    ci,
						useDocker:     docker,
						interactive:   true,
					})
					return nil
				})
				require.NoError(t, err)
				plain := ansi.Strip(output)
				assert.Contains(t, plain, "shopware-cli project validate .")
				assert.Contains(t, plain, "shopware-cli extension fix custom/plugins/MyPlugin --dry-run")
				assert.Contains(t, plain, "shopware-cli project dump --output dump.sql")
				assert.Equal(t, docker, strings.Contains(plain, "http://127.0.0.1:8000"))
				assert.Equal(t, ci == shop.CIGitHub, strings.Contains(plain, ".github/workflows/ci.yml"))
				assert.Equal(t, ci == shop.CIGitLab, strings.Contains(plain, ".gitlab-ci.yml"))
				assert.NotContains(t, plain, "shopware-cli project deploy")
			})
		}
	}
}
