//go:build deployment

package project

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

type rolloutFakeBackend struct {
	deploymentTestBackend
	result   deployment.Rollout
	err      error
	received deployment.Deployment
	logs     string
}

func (f *rolloutFakeBackend) RolloutDeployment(_ context.Context, artifact deployment.Deployment, output io.Writer) (deployment.Rollout, error) {
	_, _ = io.WriteString(output, f.logs)
	f.received = artifact
	return f.result, f.err
}

func TestProjectDeploymentRollout(t *testing.T) {
	artifact := deployment.Deployment{Reference: "./builds/shop.tar.gz"}
	for _, tc := range []struct {
		name, reference, wantErr string
		err                      error
	}{
		{"success", "release-id", "", nil},
		{"failed", "", "roll out deployment: prepare failed", errors.New("prepare failed")},
		{"cancelled", "", "roll out deployment: context canceled", context.Canceled},
		{"empty", "", "backend returned an empty rollout reference", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())
			var out bytes.Buffer
			cmd.SetOut(&out)
			fake := &rolloutFakeBackend{result: deployment.Rollout{Reference: tc.reference}, err: tc.err}
			err := runProjectDeploymentRollout(cmd, fake, artifact)
			assert.Equal(t, artifact, fake.received)
			if tc.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, "Deployed \"./builds/shop.tar.gz\" successfully\n", out.String())
			} else {
				require.ErrorContains(t, err, tc.wantErr)
				assert.Empty(t, out.String())
			}
		})
	}
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	var reference bytes.Buffer
	cmd.SetOut(&reference)
	var logs bytes.Buffer
	cmd.SetErr(&logs)
	require.NoError(t, runProjectDeploymentRollout(cmd, &rolloutFakeBackend{
		result: deployment.Rollout{Reference: "id"},
		logs:   "preparing\nactivating\n",
	}, artifact))
	assert.Equal(t, "Deployed \"./builds/shop.tar.gz\" successfully\n", reference.String())
	assert.Equal(t, "preparing\nactivating\n", logs.String())
	reference.Reset()
	require.NoError(t, runProjectDeploymentRollout(cmd, &rolloutFakeBackend{
		result: deployment.Rollout{Reference: "existing-id", Unchanged: true, Active: true},
	}, deployment.Deployment{Reference: "happy-euclid"}))
	assert.Equal(t, "Deployment \"happy-euclid\" is already active; nothing to do\n", reference.String())
	writeErr := errors.New("output closed")
	cmd.SetOut(deploymentErrorWriter{err: writeErr})
	require.ErrorIs(t, runProjectDeploymentRollout(cmd, &rolloutFakeBackend{result: deployment.Rollout{Reference: "id"}}, artifact), writeErr)
}

func newLifecycleCommand(t *testing.T, args []string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	oldConfig, oldEnv := projectConfigPath, environmentName
	t.Cleanup(func() { projectConfigPath, environmentName = oldConfig, oldEnv })
	root := &cobra.Command{Use: "project", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&projectConfigPath, "project-config", "", "")
	root.PersistentFlags().StringVarP(&environmentName, "env", "e", "", "")
	deployment := &cobra.Command{Use: "deploy"}
	create := &cobra.Command{Use: projectDeploymentCreateCmd.Use, Args: projectDeploymentCreateCmd.Args, RunE: projectDeploymentCreateCmd.RunE}
	create.Flags().StringP("output", "o", "", "")
	create.Flags().Bool("with-dev-dependencies", false, "")
	initialize := &cobra.Command{Use: projectDeploymentInitCmd.Use, Args: projectDeploymentInitCmd.Args, RunE: projectDeploymentInitCmd.RunE}
	rollout := &cobra.Command{Use: projectDeploymentRolloutCmd.Use, Args: projectDeploymentRolloutCmd.Args, RunE: projectDeploymentRolloutCmd.RunE}
	logs := &cobra.Command{Use: projectDeploymentLogsCmd.Use, Args: projectDeploymentLogsCmd.Args, RunE: projectDeploymentLogsCmd.RunE}
	prune := &cobra.Command{Use: projectDeploymentPruneCmd.Use, Args: projectDeploymentPruneCmd.Args, RunE: projectDeploymentPruneCmd.RunE}
	prune.Flags().Int("keep", 5, "")
	prune.Flags().Bool("dry-run", false, "")
	deployment.AddCommand(create, initialize, rollout, logs, prune)
	root.AddCommand(deployment)
	out := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs(append([]string{"deploy"}, args...))
	return root, out
}

func TestDeploymentLifecycleCommandsRegistered(t *testing.T) {
	for _, name := range []string{"create", "init", "list", "logs", "prune", "rollout"} {
		cmd, remaining, err := projectRootCmd.Find([]string{"deploy", name})
		require.NoError(t, err)
		assert.Empty(t, remaining)
		assert.Equal(t, name, cmd.Name())
	}
}

func TestSSHDeploymentCreateCommandUsesProjectConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a PHP shell stand-in")
	}
	bin := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(bin, "php"), "#!/bin/sh\n[ \"$1\" = bin/ci ] || exit 1\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "php"), 0o755))
	testhelper.WriteFile(t, filepath.Join(bin, "ssh"), "#!/bin/sh\nexit 99\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "ssh"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PHP_BINARY", "")
	t.Setenv("PROJECT_ROOT", "")
	for _, mode := range []string{"closest project", "explicit directory", "custom config"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			caller := t.TempDir()
			t.Chdir(caller)
			testhelper.WriteFile(t, filepath.Join(root, "composer.json"), `{"require":{"shopware/core":"6.7.0.0"}}`)
			testhelper.WriteFile(t, filepath.Join(root, "bin/console"), "<?php")
			configPath := filepath.Join(root, ".shopware-project.yaml")
			args := []string{"create", root, "-e", "production"}
			switch mode {
			case "closest project":
				t.Chdir(filepath.Join(root, "bin"))
				args = []string{"create", "-e", "production"}
			case "custom config":
				configPath = filepath.Join(caller, "custom.yml")
				args = append(args, "--project-config", "custom.yml")
			}
			testhelper.WriteFile(t, configPath, `
compatibility_date: "2026-01-01"
disable_composer_install: true
build:
  disable_checksums: true
environments:
  production:
    type: ssh
    ssh:
      host: example.invalid
      directory: /var/www/shop/current
`)
			cmd, out := newLifecycleCommand(t, args)
			require.NoError(t, cmd.ExecuteContext(t.Context()))
			output := strings.TrimSpace(out.String())
			require.True(t, strings.HasPrefix(output, `Created deployment "`))
			reference := strings.TrimSuffix(strings.TrimPrefix(output, `Created deployment "`), `"`)
			assert.Regexp(t, `^[a-z]+-[a-z]+-[a-z]+$`, reference)
			assert.FileExists(t, filepath.Join(root, ".shopware-cli", "deployments", reference+".tar.gz"))
		})
	}
}

func TestProjectDeploymentRolloutCommandValidation(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("PROJECT_ROOT", "")
	t.Setenv("SHOPWARE_CLI_NO_SYMFONY_CLI", "1")
	testhelper.WriteFile(t, filepath.Join(root, "composer.json"), `{"require":{"shopware/core":"6.7.0.0"}}`)
	testhelper.WriteFile(t, filepath.Join(root, "bin/console"), "<?php")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"rollout"}, "accepts 1 arg"},
		{[]string{"rollout", "one", "two"}, "accepts 1 arg"},
		{[]string{"rollout", "archive", "-e", "missing"}, `environment "missing" not found`},
		{[]string{"rollout", "archive"}, `deployments are not supported for environment type "local"`},
		{[]string{"logs"}, "accepts 1 arg"},
		{[]string{"logs", "one", "two"}, "accepts 1 arg"},
		{[]string{"logs", "archive", "-e", "missing"}, `environment "missing" not found`},
		{[]string{"logs", "archive"}, `deployments are not supported for environment type "local"`},
		{[]string{"prune", "extra"}, `unknown command "extra"`},
		{[]string{"prune", "--keep", "-1"}, "--keep must not be negative"},
		{[]string{"prune", "-e", "missing"}, `environment "missing" not found`},
		{[]string{"prune"}, `deployments are not supported for environment type "local"`},
	} {
		cmd, out := newLifecycleCommand(t, tc.args)
		require.ErrorContains(t, cmd.ExecuteContext(t.Context()), tc.want)
		assert.Empty(t, out.String())
	}
}

func TestProjectDeploymentRolloutValidatesSharedConfig(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("PROJECT_ROOT", "")
	t.Setenv("PATH", t.TempDir()) // Invalid configuration must fail before SSH.
	testhelper.WriteFile(t, filepath.Join(root, "composer.json"), `{"require":{"shopware/core":"6.7.0.0"}}`)
	testhelper.WriteFile(t, filepath.Join(root, "bin/console"), "<?php")
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-project.yml"), `
compatibility_date: "2026-01-01"
environments:
  production:
    type: ssh
    ssh:
      host: example.invalid
      directory: /var/www/shop/current
      shared:
        directories: [../outside]
`)
	cmd, out := newLifecycleCommand(t, []string{"rollout", "unused.tar.gz", "-e", "production"})
	require.ErrorContains(t, cmd.ExecuteContext(t.Context()), "ssh.shared")
	assert.Empty(t, out.String())
}
