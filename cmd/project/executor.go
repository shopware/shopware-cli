package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

// readConfigWithEnvironment loads the project config with the -e/--env
// environment applied, so Admin API commands target the selected environment.
// Without -e the config is returned as-is: Admin API commands historically
// used the base url/admin_api, and an existing environments.local entry must
// not silently retarget them.
func readConfigWithEnvironment(cmd *cobra.Command, allowFallback bool) (*shop.Config, error) {
	cfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, allowFallback)
	if err != nil {
		return nil, err
	}

	if environmentName == "" {
		return cfg, nil
	}

	return cfg.WithEnvironment(environmentName)
}

// resolveExecutor returns the Executor for the current environment.
func resolveExecutor(cmd *cobra.Command, projectRoot string) (executor.Executor, error) {
	cfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
	if err != nil {
		return nil, err
	}

	envCfg, err := cfg.ResolveEnvironment(environmentName)
	if err != nil {
		return nil, err
	}

	return executor.New(projectRoot, envCfg, cfg)
}
