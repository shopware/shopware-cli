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
)

//go:embed ssh_deployment_init.php
var sshDeploymentInitScript string

// initializationStore separates configuration storage from prompting and group coordination.
type initializationStore interface {
	Inspect(context.Context) (sshDeploymentInitState, error)
	Apply(context.Context, sshDeploymentInitConfig) error
}

type sshInitializationStore struct {
	ssh *SSH
}

type sshDeploymentInitState struct {
	Initialized        bool     `json:"initialized"`
	HasSharedDirectory bool     `json:"has_shared_directory"`
	RuntimeSHA256      string   `json:"runtime_sha256"`
	HasCurrent         bool     `json:"has_current"`
	HasRuntimeConfig   bool     `json:"has_runtime_config"`
	HasInstallConfig   bool     `json:"has_install_config"`
	RuntimeKeys        []string `json:"runtime_keys"`
	InstallKeys        []string `json:"install_keys"`
}

// HasValidRuntimeConfig reports whether inspection found a runtime file and its content digest.
func (s sshDeploymentInitState) HasValidRuntimeConfig() bool {
	return s.HasRuntimeConfig && s.RuntimeSHA256 != ""
}

type sshDeploymentInitConfig struct {
	RuntimeValues map[string]string
	InstallValues map[string]string
}

type sshDeploymentInitInput struct {
	Action        string            `json:"action"`
	Root          string            `json:"root"`
	Current       string            `json:"current,omitempty"`
	RuntimeValues map[string]string `json:"runtime_values,omitempty"`
	InstallValues map[string]string `json:"install_values,omitempty"`
}

// Inspect reads remote initialization metadata without returning configuration secrets.
func (s sshInitializationStore) Inspect(ctx context.Context) (sshDeploymentInitState, error) {
	var state sshDeploymentInitState
	root, err := s.ssh.deploymentRoot()
	if err != nil {
		return state, err
	}
	output, err := s.run(ctx, sshDeploymentInitInput{Action: "inspect", Root: root, Current: s.ssh.activeLinkName()})
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

// Apply writes runtime and install values through the protected remote initialization script.
func (s sshInitializationStore) Apply(ctx context.Context, config sshDeploymentInitConfig) error {
	root, err := s.ssh.deploymentRoot()
	if err != nil {
		return err
	}
	if len(config.RuntimeValues) == 0 && len(config.InstallValues) == 0 {
		return errors.New("deployment initialization has no values to write")
	}
	_, err = s.run(ctx, sshDeploymentInitInput{
		Action: "apply", Root: root, Current: s.ssh.activeLinkName(), RuntimeValues: config.RuntimeValues, InstallValues: config.InstallValues,
	})
	return err
}

// run sends initialization input over stdin so secrets stay out of SSH command arguments.
func (s sshInitializationStore) run(ctx context.Context, input sshDeploymentInitInput) ([]byte, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	cmd := s.ssh.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(withSSHDeploymentLayout(sshDeploymentInitScript), "<?php\n")).Cmd
	cmd.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("initialize SSH deployment: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
