// Package deployment provides an abstraction for deploying a Shopware project
// to a remote environment. Deployment methods (SSH, SFTP, PaaS, ...) implement
// the Deployer interface so the CLI can offer the same commands for all of them.
package deployment

import (
	"context"
	"fmt"

	"github.com/shopware/shopware-cli/internal/shop"
)

// Release describes a single release on the deployment target.
type Release struct {
	// Name of the release (sortable, e.g. a timestamp)
	Name string
	// Active is true when the release is the one currently served
	Active bool
	// Bad is true when the release was marked as bad by a rollback
	Bad bool
}

// Options controls a single deployment run.
type Options struct {
	// SkipBuildHooks skips the local build hooks before the upload
	SkipBuildHooks bool
}

// Deployer is the common interface of all deployment methods.
type Deployer interface {
	// Deploy uploads the project as a new release and switches to it
	Deploy(ctx context.Context, opts Options) error
	// Rollback switches back to a previous release. When release is empty,
	// the release deployed before the currently active one is used.
	Rollback(ctx context.Context, release string) error
	// Releases lists the releases available on the target
	Releases(ctx context.Context) ([]Release, error)
	// Close releases all resources held by the deployer
	Close() error
}

// NewDeployer creates the Deployer matching the environment type.
func NewDeployer(projectRoot string, env *shop.EnvironmentConfig, cfg *shop.Config) (Deployer, error) {
	switch env.Type {
	case "ssh":
		return newSSHDeployer(projectRoot, env, cfg)
	default:
		return nil, fmt.Errorf("environment type %q does not support deployments, supported types: ssh", env.Type)
	}
}
