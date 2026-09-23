package deployment

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

// WriteDeploymentLogs streams stored helper output from the remote host.
// It does not require the original local archive or an active release.
func (s *SSH) WriteDeploymentLogs(ctx context.Context, deployment Deployment, output io.Writer) error {
	root, err := s.deploymentRoot()
	if err != nil {
		return err
	}
	release, err := sshDeploymentReleaseName(deployment.Reference)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		Root    string `json:"root"`
		Release string `json:"release"`
	}{Root: root, Release: release})
	if err != nil {
		return err
	}
	if output == nil {
		output = io.Discard
	}
	cmd := s.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(sshDeploymentLogsScript, "<?php\n"), "--", string(payload)).Cmd
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
