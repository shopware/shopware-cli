//go:build deployment

package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/shop"
)

func resolveProjectDeploymentBackend(cmd *cobra.Command, root string) (deployment.Backend, error) {
	configPath := packageProjectConfigPath(cmd, root)
	cfg, err := shop.ReadConfig(cmd.Context(), configPath, true)
	if err != nil {
		return nil, err
	}
	env, err := cfg.ResolveEnvironment(environmentName)
	if err != nil {
		return nil, err
	}
	return deployment.New(root, configPath, cfg, env)
}
