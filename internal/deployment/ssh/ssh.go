package ssh

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/projectbuild"
	"github.com/shopware/shopware-cli/internal/shop"
)

type SSH struct {
	name       string
	initStore  initializationStore
	transport  *executor.SSHExecutor
	directory  string
	root       string
	configPath string
	config     *shop.Config
	env        *shop.EnvironmentConfig
}

var _ deployment.Backend = (*SSH)(nil)

func (s *SSH) Type() string { return executor.TypeSSH }

// New constructs an SSH deployment backend for the resolved environment.
func New(root, configPath string, cfg *shop.Config, env *shop.EnvironmentConfig) (deployment.Backend, error) {
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
			config: cfg, env: &selected, directory: host.Config.Directory, name: host.Name,
		}
		backend.initStore = sshInitializationStore{ssh: backend}
		members = append(members, sshHost{name: host.Name, backend: backend})
	}
	if len(members) == 1 {
		return members[0].backend, nil
	}
	return newSSHGroup(members, migrationHost, env.SSH.Parallelism)
}

// CreateDeployment builds locally and retains the archive for later rollout.
// Default output uses a generated deployment name; custom output uses its path.
func (s *SSH) CreateDeployment(ctx context.Context, options deployment.CreateOptions) (deployment.Deployment, error) {
	archive := projectbuild.ArchiveOptions{
		ConfigPath: s.configPath,
		OutputPath: options.OutputPath,
		Build:      projectbuild.Options{WithDevDependencies: options.WithDevDependencies, ToolVersion: options.ToolVersion},
	}
	reference, err := projectbuild.PackageArchive(ctx, s.root, s.config, s.env, archive)
	if err != nil {
		return deployment.Deployment{}, fmt.Errorf("cannot create deployment archive: %w", err)
	}
	if options.OutputPath == "" {
		reference = strings.TrimSuffix(filepath.Base(reference), ".tar.gz")
	}
	return deployment.Deployment{Reference: reference}, nil
}
