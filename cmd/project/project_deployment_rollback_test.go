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
