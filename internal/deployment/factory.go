package deployment

import (
	"context"
	"fmt"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

// ResolveBackend loads the project config and selects a backend for the environment.
// An empty configPath searches for the config in the project root.
func ResolveBackend(ctx context.Context, root, configPath, environmentName string) (Backend, error) {
	allowFallback := configPath == ""
	if allowFallback {
		configPath = shop.SearchConfigPath(ctx, root, "")
	}
	cfg, err := shop.ReadConfig(ctx, configPath, allowFallback)
	if err != nil {
		return nil, err
	}
	env, err := cfg.ResolveEnvironment(environmentName)
	if err != nil {
		return nil, err
	}
	return New(root, configPath, cfg, env)
}

// New selects a deployment backend independently of general command execution.
func New(root, configPath string, cfg *shop.Config, env *shop.EnvironmentConfig) (Backend, error) {
	kind := executor.TypeLocal
	if env != nil && env.Type != "" {
		kind = env.Type
	}
	if kind != executor.TypeSSH {
		return nil, fmt.Errorf("cannot create deployments for environment type %q: %w", kind, ErrNotSupported)
	}
	// Validate the SSH environment without connecting to the remote host.
	if _, err := executor.New(root, env, cfg); err != nil {
		return nil, fmt.Errorf("cannot configure SSH deployment: %w", err)
	}
	return &SSH{root: root, configPath: configPath, config: cfg, env: env}, nil
}
