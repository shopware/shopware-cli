package ssh

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
	hostOptions := make([]DeploymentPruneOptions, len(g.hosts))
	for i, host := range g.hosts {
		hostOptions[i] = options
		if options.ExpectedPlan == nil {
			continue
		}
		hostOptions[i].ExpectedPlan = nil
		for _, preview := range options.ExpectedPlan.Hosts {
			if preview.Host == host.name {
				hostOptions[i].ExpectedPlan = &DeploymentPruneResult{
					Deployments: preview.Deployments, Artifacts: preview.Artifacts,
				}
				break
			}
		}
		if hostOptions[i].ExpectedPlan == nil {
			return result, fmt.Errorf("cannot prune deployments: missing confirmed preview for host %q", host.name)
		}
	}
	rows := make([]*DeploymentPruneHostResult, len(g.hosts))
	err := g.parallel(ctx, func(i int) error {
		host := g.hosts[i]
		pruned, err := host.backend.PruneDeployments(ctx, hostOptions[i])
		if err == nil || !pruned.Empty() {
			rows[i] = &DeploymentPruneHostResult{
				Host: host.name, Deployments: pruned.Deployments, Artifacts: pruned.Artifacts,
				RecentDeployments: pruned.RecentDeployments, ArtifactSkipReasons: pruned.ArtifactSkipReasons,
				IncompleteDeployments: pruned.IncompleteDeployments,
			}
		}
		if err != nil {
			return fmt.Errorf("cannot prune deployments; cleanup may be partial or unknown: %w", err)
		}
		return nil
	})
	// newSSHGroup sorts hosts; collect after all workers finish, preserving empty
	// successful results and confirmed progress from failed hosts.
	for _, row := range rows {
		if row != nil {
			result.Hosts = append(result.Hosts, *row)
		}
	}
	return result, err
}
