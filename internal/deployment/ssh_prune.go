package deployment

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

//go:embed ssh_deployment_prune.php
var sshDeploymentPruneScript string

// PruneDeployments applies retention remotely under the deployment lock.
// It never reads or removes local archives.
func (s *SSH) PruneDeployments(ctx context.Context, options DeploymentPruneOptions) (DeploymentPruneResult, error) {
	var result DeploymentPruneResult
	if options.Keep < 0 {
		return result, errors.New("deployment retention count must not be negative")
	}
	root, err := s.deploymentRoot()
	if err != nil {
		return result, err
	}
	payload, err := json.Marshal(struct {
		Root string `json:"root"`
		DeploymentPruneOptions
	}{Root: root, DeploymentPruneOptions: options})
	if err != nil {
		return result, err
	}
	cmd := s.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(sshDeploymentPruneScript, "<?php\n"), "--", string(payload)).Cmd
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		err = errors.Join(ctx.Err(), err)
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return result, fmt.Errorf("prune SSH deployments: %s: %w", message, err)
		}
		return result, fmt.Errorf("prune SSH deployments: %w", err)
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return DeploymentPruneResult{}, fmt.Errorf("decode SSH prune result: %w", err)
	}
	return result, nil
}
