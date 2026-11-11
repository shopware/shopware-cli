package backend

import (
	"context"
	"fmt"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/deployment/ssh"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

// Deployment constructors define the supported environment types for both
// backend resolution and interactive environment selection.
var constructors = map[string]func(string, string, *shop.Config, *shop.EnvironmentConfig) (deployment.Backend, error){
	executor.TypeSSH: ssh.New,
}

// ResolveBackend loads the project config and selects a backend for the environment.
// An empty configPath searches for the config in the project root.
func ResolveBackend(ctx context.Context, root, configPath, environmentName string) (deployment.Backend, error) {
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
func New(root, configPath string, cfg *shop.Config, env *shop.EnvironmentConfig) (deployment.Backend, error) {
	kind := executor.TypeLocal
	if env != nil && env.Type != "" {
		kind = env.Type
	}
	constructor, ok := constructors[kind]
	if !ok {
		return nil, fmt.Errorf("cannot create deployments for environment type %q: %w", kind, deployment.ErrNotSupported)
	}
	return constructor(root, configPath, cfg, env)
}
