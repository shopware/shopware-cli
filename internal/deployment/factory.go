package deployment

import (
	"fmt"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

// New selects a deployment backend independently of general command execution.
func New(root, configPath string, cfg *shop.Config, env *shop.EnvironmentConfig) (Backend, error) {
	kind := executor.TypeLocal
	if env != nil && env.Type != "" {
		kind = env.Type
	}
	if kind != executor.TypeSSH {
		return nil, fmt.Errorf("deployments are not supported for environment type %q: %w", kind, ErrNotSupported)
	}
	target, err := executor.New(root, env, cfg)
	if err != nil {
		return nil, err
	}
	return &SSH{
		transport: target.(*executor.SSHExecutor), root: root, configPath: configPath,
		config: cfg, env: env, directory: env.SSH.Directory,
	}, nil
}
