package deployment

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/projectbuild"
	"github.com/shopware/shopware-cli/internal/shop"
)

type SSH struct {
	transport  *executor.SSHExecutor
	root       string
	configPath string
	config     *shop.Config
	env        *shop.EnvironmentConfig
	directory  string
}

var _ Backend = (*SSH)(nil)
var _ RolloutHistory = (*SSH)(nil)
var _ DeploymentLogs = (*SSH)(nil)
var _ DeploymentPruner = (*SSH)(nil)

func (s *SSH) Type() string { return executor.TypeSSH }

// CreateDeployment builds locally and retains the archive for repeated rollouts.
func (s *SSH) CreateDeployment(ctx context.Context, options CreateOptions) (Deployment, error) {
	archive := projectbuild.ArchiveOptions{
		ConfigPath: s.configPath,
		OutputPath: options.OutputPath,
		Build:      projectbuild.Options{WithDevDependencies: options.WithDevDependencies, ToolVersion: options.ToolVersion},
	}
	reference, err := projectbuild.PackageArchive(ctx, s.root, s.config, s.env, archive)
	if err != nil {
		return Deployment{}, err
	}
	if options.OutputPath == "" {
		reference = strings.TrimSuffix(filepath.Base(reference), ".tar.gz")
	}
	return Deployment{Reference: reference}, nil
}
