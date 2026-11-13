package ssh

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"strings"

	"github.com/shopware/shopware-cli/internal/phar"
)

//go:embed php
var deploymentPHP embed.FS

// deploymentPayload packages the PHP sources with php/commands/<command>.php as entrypoint.
func deploymentPayload(command string) (*phar.Payload, error) {
	source, err := fs.Sub(deploymentPHP, "php")
	if err != nil {
		return nil, fmt.Errorf("cannot read deployment PHP files: %w", err)
	}
	return phar.Build(source, "commands/"+command+".php")
}

// deploymentCommand prepares the remote PHP process for a packaged command.
// The returned archive must be the first bytes written to its stdin; any
// command input follows it on the same stream.
func (s *SSH) deploymentCommand(ctx context.Context, command string, args ...string) (*exec.Cmd, io.Reader, error) {
	payload, err := deploymentPayload(command)
	if err != nil {
		return nil, nil, err
	}
	return s.transport.RemotePHPCommand(ctx, payload.PHPArgs(args...)...).Cmd, payload.Stdin(), nil
}

// runDeploymentCommand runs a packaged command to completion and writes its
// stdout to output. Options are sent as JSON: as an argument by default, or
// after the archive on stdin when they are secret. A failure includes what the
// remote command reported on stderr.
func (s *SSH) runDeploymentCommand(ctx context.Context, command string, options any, secret bool, output io.Writer) error {
	data, err := json.Marshal(options)
	if err != nil {
		return err
	}
	var args []string
	if !secret {
		args = []string{string(data)}
	}
	cmd, archive, err := s.deploymentCommand(ctx, command, args...)
	if err != nil {
		return err
	}
	cmd.Stdin = archive
	if secret {
		cmd.Stdin = io.MultiReader(archive, bytes.NewReader(data))
	}
	cmd.Stdout = output
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		err = errors.Join(ctx.Err(), err)
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return fmt.Errorf("%s: %w", message, err)
		}
		return err
	}
	return nil
}

// deploymentCommandOutput returns the stdout of a packaged command. Output
// written before a failure is returned together with the error.
func (s *SSH) deploymentCommandOutput(ctx context.Context, command string, options any) ([]byte, error) {
	var stdout bytes.Buffer
	err := s.runDeploymentCommand(ctx, command, options, false, &stdout)
	return stdout.Bytes(), err
}
