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

func TestDeploymentBackendHostSelection(t *testing.T) {
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
`)
	environmentName = "production"
	projectConfigPath = config
	for _, tc := range []struct {
		command, host, wantErr string
		single                 bool
	}{
		{"rollout", "", "", false},
		{"rollback", "", "", false},
		{"create", "", "", false},
		{"logs", "", "", false},
		{"list", "web-2", "", true},
		{"logs", "web-2", "", true},
		{"init", "web-1", "", true},
		{"prune", "web-2", "", true},
		{"rollout", "web-1", "targets the whole environment", false},
		{"rollback", "web-1", "targets the whole environment", false},
		{"create", "web-1", "targets the whole environment", false},
		{"init", "", "requires --ssh-host", false},
		{"prune", "", "requires --ssh-host", false},
		{"logs", "missing", "unknown SSH host", false},
	} {
		t.Run(tc.command+"/"+tc.host, func(t *testing.T) {
			cmd := &cobra.Command{Use: tc.command}
			cmd.SetContext(t.Context())
			cmd.Flags().String("project-config", "", "")
			cmd.Flags().String("ssh-host", tc.host, "")
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
