package deployment

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/internal/shop"
)

//go:embed ssh_deployment_init.php
var sshDeploymentInitScript string

type sshDeploymentInitState struct {
	HasCurrent       bool     `json:"has_current"`
	HasRuntimeConfig bool     `json:"has_runtime_config"`
	HasInstallConfig bool     `json:"has_install_config"`
	RuntimeKeys      []string `json:"runtime_keys"`
	InstallKeys      []string `json:"install_keys"`
}

type sshDeploymentInitConfig struct {
	RuntimeValues map[string]string
	InstallValues map[string]string
}

type sshDeploymentInitInput struct {
	Action        string            `json:"action"`
	Root          string            `json:"root"`
	RuntimeValues map[string]string `json:"runtime_values,omitempty"`
	InstallValues map[string]string `json:"install_values,omitempty"`
}

func (s *SSH) inspectDeploymentInitialization(ctx context.Context) (sshDeploymentInitState, error) {
	var state sshDeploymentInitState
	root, err := s.deploymentRoot()
	if err != nil {
		return state, err
	}
	output, err := s.runSSHDeploymentInitialization(ctx, sshDeploymentInitInput{Action: "inspect", Root: root})
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(output, &state); err != nil {
		return state, fmt.Errorf("parse SSH deployment initialization state: %w", err)
	}
	slices.Sort(state.RuntimeKeys)
	slices.Sort(state.InstallKeys)
	return state, nil
}

func (s *SSH) applyDeploymentInitialization(ctx context.Context, config sshDeploymentInitConfig) error {
	root, err := s.deploymentRoot()
	if err != nil {
		return err
	}
	if len(config.RuntimeValues) == 0 && len(config.InstallValues) == 0 {
		return errors.New("deployment initialization has no values to write")
	}
	_, err = s.runSSHDeploymentInitialization(ctx, sshDeploymentInitInput{
		Action: "apply", Root: root, RuntimeValues: config.RuntimeValues, InstallValues: config.InstallValues,
	})
	return err
}

func (s *SSH) runSSHDeploymentInitialization(ctx context.Context, input sshDeploymentInitInput) ([]byte, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	cmd := s.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(sshDeploymentInitScript, "<?php\n")).Cmd
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("initialize SSH deployment: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func (s *SSH) sharedFiles() ([]string, error) {
	var config *shop.EnvironmentSSHConfig
	if s.env != nil {
		config = s.env.SSH
	}
	files, _, err := config.SharedPaths()
	return files, err
}
