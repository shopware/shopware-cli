//go:build deployment

package project

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/system"
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
			cmd.SetContext(system.WithInteraction(t.Context(), false))
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
	cmd.SetContext(system.WithInteraction(t.Context(), false))
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
