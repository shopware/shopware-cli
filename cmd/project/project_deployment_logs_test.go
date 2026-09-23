//go:build deployment

package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

type deploymentLogsFakeBackend struct {
	deploymentTestBackend
	received deployment.Deployment
	logs     string
	err      error
}

func (f *deploymentLogsFakeBackend) WriteDeploymentLogs(_ context.Context, artifact deployment.Deployment, output io.Writer) error {
	f.received = artifact
	if f.err != nil {
		return f.err
	}
	_, err := io.WriteString(output, f.logs)
	return err
}

func TestProjectDeploymentLogs(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	var output bytes.Buffer
	cmd.SetOut(&output)
	artifact := deployment.Deployment{Reference: "happy-euclid"}
	target := &deploymentLogsFakeBackend{logs: "helper stdout\nhelper stderr\n"}
	require.NoError(t, runProjectDeploymentLogs(cmd, target, artifact))
	assert.Equal(t, artifact, target.received)
	assert.Equal(t, target.logs, output.String())

	target.err = context.Canceled
	require.ErrorIs(t, runProjectDeploymentLogs(cmd, target, artifact), context.Canceled)
	target.err = errors.New("logs not found")
	require.ErrorContains(t, runProjectDeploymentLogs(cmd, target, artifact), "read deployment logs: logs not found")
	require.ErrorIs(t, runProjectDeploymentLogs(cmd, deploymentTestBackend{}, artifact), deployment.ErrNotSupported)

	target.err = nil
	writeErr := errors.New("output closed")
	cmd.SetOut(deploymentErrorWriter{err: writeErr})
	require.ErrorIs(t, runProjectDeploymentLogs(cmd, target, artifact), writeErr)
}

func TestSSHDeploymentLogsCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SSH fixture uses a POSIX shell")
	}
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP CLI is required")
	}
	bin := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(bin, "ssh"), "#!/bin/sh\nfor arg do command=\"$arg\"; done\nexec /bin/sh -c \"$command\"\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "ssh"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PROJECT_ROOT", "")
	root := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(root, "composer.json"), `{"require":{"shopware/core":"6.7.0.0"}}`)
	testhelper.WriteFile(t, filepath.Join(root, "bin/console"), "<?php")
	t.Chdir(filepath.Join(root, "bin"))

	remote := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(remote, ".shopware-cli/logs/happy-euclid.log"), "retained helper output\n")
	configPath := filepath.Join(t.TempDir(), "custom.yml")
	testhelper.WriteFile(t, configPath, fmt.Sprintf(`
compatibility_date: "2026-01-01"
environments:
  production:
    type: ssh
    ssh:
      host: example.invalid
      directory: %q
      php_binary: %q
`, filepath.Join(remote, "current"), php))
	cmd, output := newLifecycleCommand(t, []string{"logs", "happy-euclid", "-e", "production", "--project-config", configPath})
	require.NoError(t, cmd.ExecuteContext(t.Context()))
	assert.Equal(t, "retained helper output\n", output.String())
	assert.NoFileExists(t, filepath.Join(remote, "current"))
}
