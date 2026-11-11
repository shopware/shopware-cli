package ssh

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

//go:embed ssh_deployment_logs.php
var sshDeploymentLogsScript string

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
	payload, err := json.Marshal(sshDeploymentLogsInput{Root: root, Release: release})
	if err != nil {
		return err
	}
	if output == nil {
		output = io.Discard
	}
	cmd := s.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(sshDeploymentLogsScript, phpOpeningTag), "--", string(payload)).Cmd
	cmd.Stdout = output
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		err = errors.Join(ctx.Err(), err)
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return fmt.Errorf("read SSH deployment logs: %s: %w", message, err)
		}
		return fmt.Errorf("read SSH deployment logs: %w", err)
	}
	return nil
}

func (s *SSH) DeploymentLogReferences(ctx context.Context) ([]string, error) {
	root, err := s.deploymentRoot()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(sshDeploymentLogsInput{Root: root, List: true})
	if err != nil {
		return nil, err
	}
	cmd := s.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(sshDeploymentLogsScript, "<?php\n"), "--", string(payload)).Cmd
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list deployment logs: %w", errors.Join(ctx.Err(), err))
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
