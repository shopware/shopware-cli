package ssh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// PruneDeployments applies remote retention under the deployment lock.
func (s *SSH) PruneDeployments(ctx context.Context, options DeploymentPruneOptions) (DeploymentPruneResult, error) {
	var result DeploymentPruneResult
	if options.Keep < 0 {
		return result, errors.New("deployment retention count must not be negative")
	}
	root, err := s.deploymentRoot()
	if err != nil {
		return result, err
	}
	output, err := s.deploymentCommandOutput(ctx, "prune", struct {
		Root    string `json:"root"`
		Current string `json:"current"`
		DeploymentPruneOptions
	}{Root: root, Current: s.activeLinkName(), DeploymentPruneOptions: options})
	// A failed remote cleanup can still report confirmed removals.
	var decodeErr error
	if len(output) > 0 {
		decodeErr = json.Unmarshal(output, &result)
		if decodeErr != nil {
			result = DeploymentPruneResult{}
		}
	} else if err == nil {
		decodeErr = errors.New("empty response")
	}
	if err != nil {
		return result, fmt.Errorf("cannot prune SSH deployments: %w", errors.Join(err, decodeErr))
	}
	if decodeErr != nil {
		return DeploymentPruneResult{}, fmt.Errorf("cannot decode SSH prune result: %w", decodeErr)
	}
	return result, nil
}
