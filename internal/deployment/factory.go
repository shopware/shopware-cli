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
		return nil, fmt.Errorf("cannot create deployments for environment type %q: %w", kind, ErrNotSupported)
	}
	hosts, migrationHost, err := env.SSH.ResolveHosts()
	if err != nil {
		return nil, err
	}
	members := make([]sshHost, 0, len(hosts))
	for _, host := range hosts {
		selected := *env
		selected.SSH = host.Config
		target, err := executor.New(root, &selected, cfg)
		if err != nil {
			return nil, err
		}
		backend := &SSH{
			transport: target.(*executor.SSHExecutor), root: root, configPath: configPath,
			config: cfg, env: &selected, directory: host.Config.Directory,
		}
		backend.initStore = sshInitializationStore{ssh: backend}
		members = append(members, sshHost{name: host.Name, backend: backend})
	}
	if len(members) == 1 {
		return members[0].backend, nil
	}
	return newSSHGroup(members, migrationHost, env.SSH.Parallelism)
}
