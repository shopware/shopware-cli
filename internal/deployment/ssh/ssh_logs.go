package ssh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

type sshDeploymentLogsInput struct {
	List    bool   `json:"list,omitempty"`
	Root    string `json:"root"`
	Release string `json:"release"`
}

// WriteDeploymentLogs streams remote helper logs without a local archive.
func (s *SSH) WriteDeploymentLogs(ctx context.Context, deployment Deployment, output io.Writer) error {
	root, err := s.deploymentRoot()
	if err != nil {
		return err
	}
	release, err := sshDeploymentReleaseName(deployment.Reference)
	if err != nil {
		return err
	}
	if output == nil {
		output = io.Discard
	}
	if err := s.runDeploymentCommand(ctx, "logs", sshDeploymentLogsInput{Root: root, Release: release}, false, output); err != nil {
		return fmt.Errorf("read SSH deployment logs: %w", err)
	}
	return nil
}

func (s *SSH) DeploymentLogReferences(ctx context.Context) ([]string, error) {
	root, err := s.deploymentRoot()
	if err != nil {
		return nil, err
	}
	data, err := s.deploymentCommandOutput(ctx, "logs", sshDeploymentLogsInput{Root: root, List: true})
	if err != nil {
		return nil, fmt.Errorf("cannot list deployment logs: %w", err)
	}
	var references []string
	if err := json.Unmarshal(data, &references); err != nil {
		return nil, fmt.Errorf("cannot decode deployment log references: %w", err)
	}
	return references, nil
}

func (g *sshGroup) DeploymentLogReferences(ctx context.Context) ([]string, error) {
	return g.hosts[g.migration].backend.DeploymentLogReferences(ctx)
}
