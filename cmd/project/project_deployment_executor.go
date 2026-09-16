//go:build deployment

package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/projectbuild"
	"github.com/shopware/shopware-cli/internal/shop"
)

func resolveProjectDeploymentExecutor(cmd *cobra.Command, root string, archive projectbuild.ArchiveOptions) (executor.Executor, error) {
	configPath := packageProjectConfigPath(cmd, root)
	cfg, err := shop.ReadConfig(cmd.Context(), configPath, true)
	if err != nil {
		return nil, err
	}
	env, err := cfg.ResolveEnvironment(environmentName)
	if err != nil {
		return nil, err
	}
	target, err := executor.New(root, env, cfg)
	if err != nil {
		return nil, err
	}
	if ssh, ok := target.(*executor.SSHExecutor); ok {
		archive.ConfigPath = configPath
		return deployment.NewSSH(ssh, root, cfg, env, archive), nil
	}
	return target, nil
}
