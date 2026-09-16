// Package deployment composes artifact builders with backend executors.
// Keeping this composition above both packages avoids a projectbuild/executor
// import cycle.
package deployment

import (
	"context"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/projectbuild"
	"github.com/shopware/shopware-cli/internal/shop"
)

type SSH struct {
	*executor.SSHExecutor
	root    string
	config  *shop.Config
	env     *shop.EnvironmentConfig
	archive projectbuild.ArchiveOptions
}

var _ executor.DeploymentExecutor = (*SSH)(nil)

func NewSSH(ssh *executor.SSHExecutor, root string, config *shop.Config, env *shop.EnvironmentConfig, archive projectbuild.ArchiveOptions) *SSH {
	return &SSH{SSHExecutor: ssh, root: root, config: config, env: env, archive: archive}
}

// CreateDeployment deliberately uses no SSH operations. The returned archive
// remains local and can be rolled out repeatedly without rebuilding.
func (s *SSH) CreateDeployment(ctx context.Context) (executor.Deployment, error) {
	reference, err := projectbuild.PackageArchive(ctx, s.root, s.config, s.env, s.archive)
	if err != nil {
		return executor.Deployment{}, err
	}
	return executor.Deployment{Reference: reference}, nil
}
