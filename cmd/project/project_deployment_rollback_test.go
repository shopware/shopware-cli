//go:build deployment

package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

type activationFakeBackend struct {
	deploymentTestBackend
	result   deployment.Rollout
	err      error
	received deployment.Deployment
}

func (f *activationFakeBackend) ActivateDeployment(_ context.Context, artifact deployment.Deployment, output io.Writer) (deployment.Rollout, error) {
	f.received = artifact
	_, _ = io.WriteString(output, "activating remotely\n")
	return f.result, f.err
}

func TestProjectDeploymentRollback(t *testing.T) {
	artifact := deployment.Deployment{Reference: "remote-only-build"}
	for _, tc := range []struct {
		name, reference, wantErr string
		unchanged                bool
		err                      error
	}{
		{"success", "new-event", "", false, nil},
		{"already active", "existing-event", "", true, nil},
		{"failed", "", "roll back deployment: activation failed", false, errors.New("activation failed")},
		{"cancelled", "", "roll back deployment: context canceled", false, context.Canceled},
		{"invalid result", "", "backend returned an empty rollout reference", false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetContext(system.WithInteraction(t.Context(), false))
			var output, logs bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&logs)
			backend := &activationFakeBackend{
				result: deployment.Rollout{Reference: tc.reference, Unchanged: tc.unchanged},
				err:    tc.err,
			}
			err := runProjectDeploymentRollback(cmd, backend, artifact)
			assert.Equal(t, artifact, backend.received)
			assert.Equal(t, "activating remotely\n", logs.String())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.Empty(t, output.String())
				return
			}
			require.NoError(t, err)
			if tc.unchanged {
				assert.Equal(t, "Deployment \"remote-only-build\" is already active; nothing to do\n", output.String())
			} else {
				assert.Equal(t, "Reactivated deployment \"remote-only-build\" successfully\n", output.String())
			}
		})
	}
	cmd := &cobra.Command{}
	cmd.SetContext(system.WithInteraction(t.Context(), false))
	require.ErrorIs(t, runProjectDeploymentRollback(cmd, deploymentTestBackend{}, artifact), deployment.ErrNotSupported)
	expected := errors.New("output closed")
	cmd.SetOut(deploymentErrorWriter{err: expected})
	cmd.SetErr(io.Discard)
	require.ErrorIs(t, runProjectDeploymentRollback(cmd, &activationFakeBackend{result: deployment.Rollout{Reference: "id"}}, artifact), expected)
}

func TestSSHDeploymentRollbackCommandAndCompletion(t *testing.T) {
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
	testhelper.WriteFile(t, filepath.Join(bin, "hostname"), "#!/bin/sh\nprintf 'fixture.example.invalid\\n'\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "hostname"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PROJECT_ROOT", "")
	root := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(root, "composer.json"), `{"require":{"shopware/core":"*"}}`)
	testhelper.WriteFile(t, filepath.Join(root, "bin/console"), "<?php")
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/deployments/only-local.tar.gz"), "unused")
	t.Chdir(filepath.Join(root, "bin"))
	remote := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(remote, ".shopware-cli/deployment.lock"), "")
	for i, name := range []string{"focused-wise-turing", "happy-bright-euclid"} {
		timestamp := fmt.Sprintf("2026-09-22T11:%02d:00Z", i)
		reference := name + "-event"
		checksum := strings.Repeat("a", 64)
		testhelper.WriteFile(t, filepath.Join(remote, "releases", name, "helper-marker"), "prepared once")
		testhelper.WriteFile(t, filepath.Join(remote, ".shopware-cli/logs", name+".log"), "original helper logs")
		for relative, value := range map[string]map[string]any{
			".shopware-cli/releases/" + name + ".json": {
				"release": name, "reference": reference, "sha256": checksum, "ready": true, "deployed_at": timestamp,
			},
			".shopware-cli/rollouts/" + reference + ".json": {
				"release": name, "reference": reference, "deployment": name, "sha256": checksum,
				"archive": "/no-local-archive/" + name + ".tar.gz", "status": "successful",
				"created_at": timestamp, "deployed_at": timestamp,
			},
		} {
			data, err := json.Marshal(value)
			require.NoError(t, err)
			testhelper.WriteFile(t, filepath.Join(remote, relative), string(data))
		}
	}
	require.NoError(t, os.Symlink("releases/happy-bright-euclid", filepath.Join(remote, "current")))
	config := filepath.Join(t.TempDir(), "custom.yml")
	testhelper.WriteFile(t, config, fmt.Sprintf(`
compatibility_date: "2026-01-01"
environments:
  production:
    type: ssh
    ssh:
      host: example.invalid
      directory: %q
      php_binary: %q
`, filepath.Join(remote, "current"), php))
	cmd, _ := newLifecycleCommand(t, nil)
	cmd.SetContext(t.Context())
	require.NoError(t, cmd.ParseFlags([]string{"--project-config", config, "-e", "production"}))
	references, directive := rollbackReferenceCompletions(cmd, nil, "")
	assert.ElementsMatch(t, []string{"focused-wise-turing", "happy-bright-euclid"}, references)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	references, directive = rollbackReferenceCompletions(cmd, nil, "fo")
	assert.Equal(t, []string{"focused-wise-turing"}, references)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	local, _ := deploymentReferenceCompletions(cmd, nil, "")
	assert.Equal(t, []string{"only-local"}, local)

	cmd, output := newLifecycleCommand(t, []string{"rollback", "focused-wise-turing", "-e", "production", "--project-config", config})
	require.NoError(t, cmd.ExecuteContext(system.WithInteraction(t.Context(), false)))
	assert.Contains(t, output.String(), `Reactivated deployment "focused-wise-turing" successfully`)
	target, err := os.Readlink(filepath.Join(remote, "current"))
	require.NoError(t, err)
	assert.Equal(t, "releases/focused-wise-turing", target)
	log, err := os.ReadFile(filepath.Join(remote, ".shopware-cli/logs/focused-wise-turing.log"))
	require.NoError(t, err)
	assert.Equal(t, "original helper logs", string(log))
	assert.NoDirExists(t, filepath.Join(remote, ".shopware-cli/artifacts"))
}
