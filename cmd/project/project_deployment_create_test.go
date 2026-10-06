//go:build deployment

package project

import (
	"bytes"
	"context"
	"errors"
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

type deploymentFakeBackend struct {
	deploymentTestBackend
	deployment    deployment.Deployment
	options       deployment.CreateOptions
	err           error
	onCreate      func(context.Context)
	createCalls   int
	rolloutCalls  int
	rolledOut     deployment.Deployment
	rolloutResult deployment.Rollout
	rolloutErr    error
	rolloutLogs   string
	onRollout     func(context.Context)
}

func (f *deploymentFakeBackend) CreateDeployment(ctx context.Context, options deployment.CreateOptions) (deployment.Deployment, error) {
	f.options = options
	f.createCalls++
	if f.onCreate != nil {
		f.onCreate(ctx)
	}
	return f.deployment, f.err
}

func (f *deploymentFakeBackend) RolloutDeployment(ctx context.Context, artifact deployment.Deployment, output io.Writer) (deployment.Rollout, error) {
	f.rolloutCalls++
	f.rolledOut = artifact
	if f.onRollout != nil {
		f.onRollout(ctx)
	}
	if _, err := io.WriteString(output, f.rolloutLogs); err != nil {
		return deployment.Rollout{}, err
	}
	return f.rolloutResult, f.rolloutErr
}

func TestProjectDeploymentCreate(t *testing.T) {
	buildErr := errors.New("build failed")
	for _, tc := range []struct {
		name      string
		reference string
		buildErr  error
		wantOut   string
		wantErr   string
	}{
		{
			name:      "archive reference",
			reference: "./builds/shopware-abc123.tar.gz",
			wantOut:   "Created deployment \"./builds/shopware-abc123.tar.gz\"\n\nDeploy it with:\n  shopware-cli project deploy rollout './builds/shopware-abc123.tar.gz'\n",
		},
		{
			name:      "opaque build ID",
			reference: "01JPAASBUILDID",
			wantOut:   "Created deployment \"01JPAASBUILDID\"\n\nDeploy it with:\n  shopware-cli project deploy rollout '01JPAASBUILDID'\n",
		},
		{
			name:      "backend error",
			reference: "must-not-print",
			buildErr:  buildErr,
			wantErr:   "create deployment: build failed",
		},
		{
			name:     "cancellation",
			buildErr: context.Canceled,
			wantErr:  "create deployment: context canceled",
		},
		{
			name:    "empty reference",
			wantErr: "create deployment: backend returned an empty deployment reference",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())
			cmd.SetOut(&out)
			fake := &deploymentFakeBackend{
				deployment: deployment.Deployment{Reference: tc.reference},
				err:        tc.buildErr,
				onCreate: func(ctx context.Context) {
					assert.Equal(t, cmd.Context(), ctx)
				},
			}

			options := deployment.CreateOptions{OutputPath: "./custom.tar.gz", WithDevDependencies: true, ToolVersion: "test"}
			err := runProjectDeploymentCreate(cmd, fake, "", options)
			assert.Equal(t, options, fake.options)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				if tc.buildErr != nil {
					assert.ErrorIs(t, err, tc.buildErr)
				}
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantOut, out.String())
			assert.Equal(t, 1, fake.createCalls)
			assert.Zero(t, fake.rolloutCalls)
		})
	}
}

type deploymentErrorWriter struct {
	err error
}

func (w deploymentErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestProjectDeploymentCreateOutputError(t *testing.T) {
	writeErr := errors.New("output closed")
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	cmd.SetOut(deploymentErrorWriter{err: writeErr})
	fake := &deploymentFakeBackend{deployment: deployment.Deployment{Reference: "build-id"}}

	require.ErrorIs(t, runProjectDeploymentCreate(cmd, fake, "", deployment.CreateOptions{}), writeErr)
	assert.Equal(t, 1, fake.createCalls)
	assert.Zero(t, fake.rolloutCalls)
}

func TestProjectDeploymentCreateAndRollout(t *testing.T) {
	for _, tc := range []struct {
		name        string
		reference   string
		createErr   error
		rolloutErr  error
		wantErr     string
		wantRollout bool
	}{
		{"success", "created-build", nil, nil, "", true},
		{"creation fails", "", errors.New("build failed"), nil, "create deployment: build failed", false},
		{"empty reference", "", nil, nil, "backend returned an empty deployment reference", false},
		{"rollout fails", "created-build", nil, errors.New("remote failed"), "roll out deployment: remote failed", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetContext(system.WithInteraction(t.Context(), false))
			cmd.Flags().Bool("rollout", true, "")
			var output, logs bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&logs)
			artifact := deployment.Deployment{Reference: tc.reference, Name: "exact created artifact"}
			backend := &deploymentFakeBackend{
				deployment: artifact, err: tc.createErr,
				rolloutResult: deployment.Rollout{Reference: "activation-id"},
				rolloutErr:    tc.rolloutErr, rolloutLogs: "deployment progress\n",
				onRollout: func(ctx context.Context) {
					assert.Equal(t, cmd.Context(), ctx)
				},
			}
			err := runProjectDeploymentCreate(cmd, backend, "", deployment.CreateOptions{})
			assert.Equal(t, 1, backend.createCalls)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "Created deployment \"created-build\"\nDeployed \"created-build\" successfully\n", output.String())
			}
			assert.NotContains(t, output.String(), "Deploy it with:")
			if tc.wantRollout {
				assert.Equal(t, 1, backend.rolloutCalls)
				assert.Equal(t, artifact, backend.rolledOut)
				assert.Equal(t, "deployment progress\n", logs.String())
				assert.Contains(t, output.String(), "Created deployment \"created-build\"\n")
			} else {
				assert.Zero(t, backend.rolloutCalls)
				assert.Empty(t, output.String())
				assert.Empty(t, logs.String())
			}
		})
	}
}

func TestNextDeploymentRolloutCommandInCurrentProject(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	cmd := &cobra.Command{}
	command, err := nextDeploymentRolloutCommand(cmd, root, "focused-wise-turing")
	require.NoError(t, err)
	assert.Equal(t, "shopware-cli project deploy rollout 'focused-wise-turing'", command)
}

func TestNextDeploymentRolloutCommandPreservesContextAndQuotesArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("suggestion uses POSIX shell quoting")
	}
	caller := t.TempDir()
	t.Chdir(caller)
	root := filepath.Join(t.TempDir(), "shop's directory")
	require.NoError(t, os.Mkdir(root, 0o755))
	realRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	config := "custom config's.yml"
	absoluteConfig, err := filepath.Abs(config)
	require.NoError(t, err)
	environment := "prod's $(touch should-not-exist)"
	reference := "-build'; touch should-not-exist; echo '"
	cmd := &cobra.Command{}
	cmd.Flags().String("env", "", "")
	cmd.Flags().String("project-config", "", "")
	require.NoError(t, cmd.Flags().Set("env", environment))
	require.NoError(t, cmd.Flags().Set("project-config", config))
	command, err := nextDeploymentRolloutCommand(cmd, root, reference)
	require.NoError(t, err)

	// Execute only a recording stub, never a real deployment command.
	bin := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(bin, "shopware-cli"), "#!/bin/sh\npwd -P\nprintf '%s\\n' \"$@\"\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "shopware-cli"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := exec.CommandContext(t.Context(), "sh", "-c", command).Output()
	require.NoError(t, err)
	assert.Equal(t, []string{
		realRoot, "project", "deploy", "rollout", "-e", environment,
		"--project-config", absoluteConfig, "--", reference,
	}, strings.Split(strings.TrimSuffix(string(output), "\n"), "\n"))
	assert.NoFileExists(t, filepath.Join(root, "should-not-exist"))
	assert.NoFileExists(t, filepath.Join(caller, "should-not-exist"))
}

func TestProjectDeploymentCreateCommand(t *testing.T) {
	previousConfigPath, previousEnvironmentName := projectConfigPath, environmentName
	t.Cleanup(func() {
		projectConfigPath, environmentName = previousConfigPath, previousEnvironmentName
	})
	t.Setenv("SHOPWARE_CLI_NO_SYMFONY_CLI", "1")
	t.Setenv("PROJECT_ROOT", "")
	workDir := t.TempDir()
	t.Chdir(workDir)
	testhelper.WriteFile(t, filepath.Join(workDir, "composer.json"), `{"require":{"shopware/core":"*"}}`)
	testhelper.WriteFile(t, filepath.Join(workDir, "bin", "console"), "<?php")
	testhelper.WriteFile(t, filepath.Join(workDir, "config.yml"), `
compatibility_date: "2026-01-01"
environments:
  local:
    type: local
  production:
    type: ssh
    ssh:
      host: example.invalid
      directory: /var/www/shopware
`)

	for _, tc := range []struct {
		name string
		args []string
		want string
		cwd  string
	}{
		{"current project", nil, `deployments are not supported for environment type "local"`, ""},
		{"parent project", nil, `deployments are not supported for environment type "local"`, filepath.Join(workDir, "bin")},
		{"no project", nil, "cannot find Shopware project", t.TempDir()},
		{"rejects extra arguments", []string{".", "."}, "accepts at most 1 arg(s), received 2", ""},
		{"missing directory", []string{"missing"}, "read project directory:", ""},
		{"file instead of directory", []string{"config.yml"}, "is not a directory", ""},
		{"default environment", []string{"."}, `deployments are not supported for environment type "local"`, ""},
		{"unknown environment", []string{".", "--env", "missing"}, `environment "missing" not found`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cwd != "" {
				t.Chdir(tc.cwd)
			}

			// Isolate flags from the global command tree.
			root := &cobra.Command{Use: "project", SilenceUsage: true, SilenceErrors: true}
			root.PersistentFlags().StringVar(&projectConfigPath, "project-config", "", "")
			root.PersistentFlags().StringVarP(&environmentName, "env", "e", "", "")
			deployment := &cobra.Command{Use: "deploy"}
			create := &cobra.Command{
				Use:  projectDeploymentCreateCmd.Use,
				Args: projectDeploymentCreateCmd.Args,
				RunE: projectDeploymentCreateCmd.RunE,
			}
			deployment.AddCommand(create)
			root.AddCommand(deployment)
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"deploy", "create", "--project-config", filepath.Join(workDir, "config.yml")}, tc.args...))

			err := root.ExecuteContext(t.Context())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Empty(t, out.String())
		})
	}
}
