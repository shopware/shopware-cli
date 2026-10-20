package deployment

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/projectbuild"
	"github.com/shopware/shopware-cli/internal/shop"
)

type SSH struct {
	initStore  initializationStore
	transport  *executor.SSHExecutor
	directory  string
	root       string
	configPath string
	config     *shop.Config
	env        *shop.EnvironmentConfig
}

var _ Backend = (*SSH)(nil)

func (s *SSH) Type() string { return executor.TypeSSH }

// CreateDeployment builds locally and retains the archive for later rollout.
// Default output uses a generated deployment name; custom output uses its path.
func (s *SSH) CreateDeployment(ctx context.Context, options CreateOptions) (Deployment, error) {
	archive := projectbuild.ArchiveOptions{
		ConfigPath: s.configPath,
		OutputPath: options.OutputPath,
		Build:      projectbuild.Options{WithDevDependencies: options.WithDevDependencies, ToolVersion: options.ToolVersion},
	}
	reference, err := projectbuild.PackageArchive(ctx, s.root, s.config, s.env, archive)
	if err != nil {
		return Deployment{}, fmt.Errorf("cannot create deployment archive: %w", err)
	}
	if options.OutputPath == "" {
		reference = strings.TrimSuffix(filepath.Base(reference), ".tar.gz")
	}
	return Deployment{Reference: reference}, nil
}
