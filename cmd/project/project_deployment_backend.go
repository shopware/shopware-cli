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
	host, _ := cmd.Flags().GetString("ssh-host")
	if env.SSH != nil && len(env.SSH.Hosts) > 1 {
		switch cmd.Name() {
		case "create", "rollout", "rollback":
			if host != "" {
				return nil, fmt.Errorf("project deploy %s targets the whole environment; omit --ssh-host", cmd.Name())
			}
		case "init", "prune":
			if host == "" {
				return nil, fmt.Errorf("project deploy %s requires --ssh-host for a multi-host environment", cmd.Name())
			}
		}
	}
	env, err = selectProjectSSHHost(cmd, env, false)
	if err != nil {
		return nil, err
	}
	return deployment.New(root, configPath, cfg, env)
}
