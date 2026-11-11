package ssh

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
	payload, err := json.Marshal(struct {
		Root    string `json:"root"`
		Current string `json:"current"`
		DeploymentPruneOptions
	}{Root: root, Current: s.activeLinkName(), DeploymentPruneOptions: options})
	if err != nil {
		return result, err
	}
	cmd := s.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(withSSHDeploymentLayout(sshDeploymentPruneScript), "<?php\n"), "--", string(payload)).Cmd
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
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
		err = errors.Join(ctx.Err(), err, decodeErr)
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return result, fmt.Errorf("cannot prune SSH deployments: %s: %w", message, err)
		}
		return result, fmt.Errorf("cannot prune SSH deployments: %w", err)
	}
	if decodeErr != nil {
		return DeploymentPruneResult{}, fmt.Errorf("cannot decode SSH prune result: %w", decodeErr)
	}
	return result, nil
}
