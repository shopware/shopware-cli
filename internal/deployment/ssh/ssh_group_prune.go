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
	rows := make([]*DeploymentPruneHostResult, len(g.hosts))
	err := g.parallel(ctx, func(i int) error {
		host := g.hosts[i]
		pruned, err := host.backend.PruneDeployments(ctx, options)
		if err == nil || len(pruned.Deployments)+len(pruned.Artifacts)+len(pruned.ArtifactSkipReasons)+len(pruned.IncompleteDeployments) > 0 {
			rows[i] = &DeploymentPruneHostResult{
				Host: host.name, Deployments: pruned.Deployments, Artifacts: pruned.Artifacts,
				ArtifactSkipReasons: pruned.ArtifactSkipReasons, IncompleteDeployments: pruned.IncompleteDeployments,
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
