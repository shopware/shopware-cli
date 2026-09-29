//go:build deployment

package project

import (
	"fmt"

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
	if env.SSH != nil && len(env.SSH.Hosts) > 1 {
		switch cmd.Name() {
		case "init", "prune":
			return nil, fmt.Errorf("project deploy %s does not support multi-host environments; configure a separate single-host environment", cmd.Name())
		}
	}
	return deployment.New(root, configPath, cfg, env)
}
