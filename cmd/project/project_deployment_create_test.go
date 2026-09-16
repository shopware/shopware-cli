//go:build deployment

package project

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

type deploymentFakeExecutor struct {
	executor.Executor
	deployment   executor.Deployment
	err          error
	onCreate     func(context.Context)
	createCalls  int
	rolloutCalls int
}

func (f *deploymentFakeExecutor) CreateDeployment(ctx context.Context) (executor.Deployment, error) {
	f.createCalls++
	if f.onCreate != nil {
		f.onCreate(ctx)
	}
	return f.deployment, f.err
}

func (f *deploymentFakeExecutor) RolloutDeployment(context.Context, executor.Deployment) (executor.Rollout, error) {
	f.rolloutCalls++
	return executor.Rollout{}, errors.New("create must not roll out a deployment")
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
			wantOut:   "./builds/shopware-abc123.tar.gz\n",
		},
		{
			name:      "opaque build ID",
			reference: "01JPAASBUILDID",
			wantOut:   "01JPAASBUILDID\n",
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
			fake := &deploymentFakeExecutor{
				deployment: executor.Deployment{Reference: tc.reference},
				err:        tc.buildErr,
				onCreate: func(ctx context.Context) {
					assert.Equal(t, cmd.Context(), ctx)
				},
			}

			err := runProjectDeploymentCreate(cmd, fake)
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

func TestProjectDeploymentCreateUnsupportedExecutor(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	cmd.SetOut(&out)

	err := runProjectDeploymentCreate(cmd, executor.NewLocal(t.TempDir()))
	require.ErrorIs(t, err, executor.ErrNotSupported)
	assert.Contains(t, err.Error(), `"local"`)
	assert.Empty(t, out.String())
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
	fake := &deploymentFakeExecutor{deployment: executor.Deployment{Reference: "build-id"}}

	require.ErrorIs(t, runProjectDeploymentCreate(cmd, fake), writeErr)
	assert.Equal(t, 1, fake.createCalls)
	assert.Zero(t, fake.rolloutCalls)
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
		{"current project", nil, `creating deployments with executor "local"`, ""},
		{"parent project", nil, `creating deployments with executor "local"`, filepath.Join(workDir, "bin")},
		{"no project", nil, "cannot find Shopware project in current directory", t.TempDir()},
		{"rejects extra arguments", []string{".", "."}, "accepts at most 1 arg(s), received 2", ""},
		{"missing directory", []string{"missing"}, "read project directory:", ""},
		{"file instead of directory", []string{"config.yml"}, "is not a directory", ""},
		{"default environment", []string{"."}, `creating deployments with executor "local"`, ""},
		{"unknown environment", []string{".", "--env", "missing"}, `environment "missing" not found`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cwd != "" {
				t.Chdir(tc.cwd)
			}

			// Use a fresh command tree to exercise flag parsing without mutating
			// the global Cobra tree shared by other command tests.
			root := &cobra.Command{Use: "project", SilenceUsage: true, SilenceErrors: true}
			root.PersistentFlags().StringVar(&projectConfigPath, "project-config", "", "")
			root.PersistentFlags().StringVarP(&environmentName, "env", "e", "", "")
			deployment := &cobra.Command{Use: "deployment"}
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
			root.SetArgs(append([]string{"deployment", "create", "--project-config", filepath.Join(workDir, "config.yml")}, tc.args...))

			err := root.ExecuteContext(t.Context())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Empty(t, out.String())
		})
	}
}
