package deployment

import (
	"context"
	"errors"
	"fmt"
)

var _ DeploymentPruner = (*sshGroup)(nil)

// PruneDeployments applies retention independently on each host. Successful
// results remain available when another host fails; cleanup is not atomic across hosts.
func (g *sshGroup) PruneDeployments(ctx context.Context, options DeploymentPruneOptions) (DeploymentPruneResult, error) {
	var result DeploymentPruneResult
	if options.Keep < 0 {
		return result, errors.New("deployment retention count must not be negative")
	}
	rows := make([]*DeploymentPruneHostResult, len(g.hosts))
	err := g.parallel(ctx, func(i int) error {
		host := g.hosts[i]
		pruned, err := host.backend.PruneDeployments(ctx, options)
		if err != nil {
			return fmt.Errorf("prune failed; cleanup may be partial or unknown: %w", err)
		}
		rows[i] = &DeploymentPruneHostResult{
			Host: host.name, Deployments: pruned.Deployments, Artifacts: pruned.Artifacts,
		}
		return nil
	})
	// newSSHGroup sorts hosts; collect after all workers finish, preserving empty
	// successful results while excluding failed or cancelled hosts.
	for _, row := range rows {
		if row != nil {
			result.Hosts = append(result.Hosts, *row)
		}
	}
	return result, err
}
