//go:build deployment

package project

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

// Unexpected lifecycle calls fail the test.
type deploymentTestBackend struct{}

func (deploymentTestBackend) Type() string { return "fake" }

func (deploymentTestBackend) CreateDeployment(context.Context, deployment.CreateOptions) (deployment.Deployment, error) {
	panic("unexpected create")
}

func (deploymentTestBackend) RolloutDeployment(context.Context, deployment.Deployment, io.Writer) (deployment.Rollout, error) {
	panic("unexpected rollout")
}

func TestDeploymentBackendEnvironmentScope(t *testing.T) {
	oldConfig, oldEnvironment := projectConfigPath, environmentName
	t.Cleanup(func() { projectConfigPath, environmentName = oldConfig, oldEnvironment })
	root := t.TempDir()
	config := filepath.Join(root, "project.yml")
	testhelper.WriteFile(t, config, `
compatibility_date: "2026-01-01"
environments:
  production:
    type: ssh
    ssh:
      user: deploy
      directory: /srv/shop/current
      migration_host: web-1
      hosts:
        web-1: {host: web1.example.com}
        web-2: {host: web2.example.com}
  maintenance:
    type: ssh
    ssh:
      host: web1.example.com
      user: deploy
      directory: /srv/shop/current
`)
	projectConfigPath = config
	for _, tc := range []struct {
		command, environment, wantErr string
		single                        bool
	}{
		{"rollout", "production", "", false},
		{"rollback", "production", "", false},
		{"create", "production", "", false},
		{"logs", "production", "", false},
		{"list", "production", "", false},
		{"init", "production", "configure a separate single-host environment", false},
		{"prune", "production", "configure a separate single-host environment", false},
		{"rollout", "maintenance", "", true},
		{"rollback", "maintenance", "", true},
		{"create", "maintenance", "", true},
		{"list", "maintenance", "", true},
		{"logs", "maintenance", "", true},
		{"init", "maintenance", "", true},
		{"prune", "maintenance", "", true},
	} {
		t.Run(tc.command+"/"+tc.environment, func(t *testing.T) {
			environmentName = tc.environment
			cmd := &cobra.Command{Use: tc.command}
			cmd.SetContext(t.Context())
			cmd.Flags().String("project-config", "", "")
			require.NoError(t, cmd.Flags().Set("project-config", config))
			backend, err := resolveProjectDeploymentBackend(cmd, root)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			_, single := backend.(*deployment.SSH)
			assert.Equal(t, tc.single, single)
		})
	}
}
