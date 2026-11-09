package ssh

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/shopware/shopware-cli/internal/deployment"
)

type sshHost struct {
	name    string
	backend *SSH
}

type sshGroup struct {
	hosts       []sshHost
	migration   int
	parallelism int
}

// newSSHGroup receives hosts validated and resolved by shop.EnvironmentSSHConfig.
func newSSHGroup(hosts []sshHost, migrationHost string, parallelism int) (*sshGroup, error) {
	if len(hosts) == 0 || parallelism < 0 {
		return nil, errors.New("cannot create SSH group: hosts and nonnegative parallelism are required")
	}
	if parallelism == 0 {
		parallelism = 2
	}
	g := &sshGroup{hosts: slices.Clone(hosts), migration: -1, parallelism: parallelism}
	slices.SortFunc(g.hosts, func(a, b sshHost) int { return strings.Compare(a.name, b.name) })
	for i, host := range g.hosts {
		if host.backend == nil {
			return nil, fmt.Errorf("cannot create SSH group: host %q has no backend", host.name)
		}
		if host.name == migrationHost {
			g.migration = i
		}
	}
	if g.migration < 0 {
		return nil, fmt.Errorf("cannot create SSH group: migration host %q is not a group member", migrationHost)
	}
	return g, nil
}

// Type identifies a group of SSH hosts as an SSH deployment backend.
func (g *sshGroup) Type() string { return "ssh" }

// CreateDeployment creates the archive using the migration host configuration.
func (g *sshGroup) CreateDeployment(ctx context.Context, options deployment.CreateOptions) (deployment.Deployment, error) {
	return g.hosts[g.migration].backend.CreateDeployment(ctx, options)
}

// parallel waits for every worker before returning; all opened sessions remain
// owned by the coordinator even when a different worker fails.
func (g *sshGroup) parallel(ctx context.Context, run func(int) error) error {
	var wg sync.WaitGroup
	limit := make(chan struct{}, g.parallelism)
	errs := make([]error, len(g.hosts))
	for i := range g.hosts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-ctx.Done():
				errs[i] = fmt.Errorf("host %s: operation cancelled; state unknown: %w", g.hosts[i].name, ctx.Err())
				return
			}
			if err := run(i); err != nil {
				errs[i] = fmt.Errorf("host %s: %w", g.hosts[i].name, err)
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}
