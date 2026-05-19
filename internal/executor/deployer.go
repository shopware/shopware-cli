package executor

import (
	"context"
	"time"
)

// Deployer drives release-based deployments (rsync into releases/{timestamp},
// atomic current symlink, shared files/dirs, rollback). Implementations live
// alongside the Executor (e.g. SSHDeployer for the ssh executor). Executors
// that do not support deployment return nil from Executor.Deployer.
type Deployer interface {
	// Deploy creates and activates a new release on the target.
	Deploy(ctx context.Context) error
	// ListReleases returns the releases known on the target, oldest first.
	ListReleases(ctx context.Context) ([]Release, error)
	// Rollback activates a previous release. If name is empty, the release
	// immediately before the current one is activated.
	Rollback(ctx context.Context, name string) error
}

// Release describes a single deployed release on the target.
type Release struct {
	// Name is the on-disk directory name inside releases/ (typically a timestamp).
	Name string
	// CreatedAt is parsed from Name when it matches the deploy timestamp format.
	CreatedAt time.Time
	// Current reports whether the current/ symlink points at this release.
	Current bool
}
