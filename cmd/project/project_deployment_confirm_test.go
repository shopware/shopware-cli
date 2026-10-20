//go:build deployment

package project

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/system"
)

func TestDeploymentCommandsConfirmBeforeActivation(t *testing.T) {
	for _, kind := range []deploymentSelectionKind{rolloutSelection, rollbackSelection} {
		for _, tc := range []struct {
			name, input string
			interactive bool
			wantCall    bool
		}{
			{"confirmed", "\x1b[A\r", true, true},
			{"declined", "\x1b[A\x1b[B\r", true, false},
			{"enter defaults to cancel", "\r", true, false},
			{"aborted", "\x03", true, false},
			{"non-interactive", "", false, true},
		} {
			t.Run(string(kind)+"/"+tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				cmd := &cobra.Command{}
				cmd.SetContext(system.WithInteraction(ctx, tc.interactive))
				cmd.SetIn(strings.NewReader(tc.input))
				cmd.Flags().String("env", "production", "")
				var output, logs bytes.Buffer
				cmd.SetOut(&output)
				cmd.SetErr(&logs)
				artifact := deployment.Deployment{Reference: "focused-wise-turing"}
				var called bool
				if kind == rolloutSelection {
					backend := &rolloutFakeBackend{result: deployment.Rollout{Reference: "event"}}
					require.NoError(t, runProjectDeploymentRollout(cmd, backend, artifact))
					called = backend.received == artifact
				} else {
					backend := &activationFakeBackend{result: deployment.Rollout{Reference: "event"}}
					require.NoError(t, runProjectDeploymentRollback(cmd, backend, artifact))
					called = backend.received == artifact
				}
				assert.Equal(t, tc.wantCall, called)
				if !tc.wantCall {
					assert.Empty(t, output.String(), "canceling must not report deployment success")
				}
				if !tc.interactive {
					assert.NotContains(t, logs.String(), "environment")
				}
			})
		}
	}
}

func TestDeploymentConfirmationShowsTarget(t *testing.T) {
	for _, environment := range []string{"", "production"} {
		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())
		cmd.Flags().String("env", environment, "")
		cmd.SetIn(strings.NewReader("2\n"))
		var output, logs bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetErr(&logs)
		confirmed := false
		form := deploymentConfirmationForm(cmd, deployment.Deployment{Reference: "opaque", Name: "Friendly deployment"}, rolloutSelection, &confirmed).WithAccessible(true)
		require.NoError(t, form.RunWithContext(t.Context()))
		assert.False(t, confirmed)
		assert.Contains(t, logs.String(), "Friendly deployment")
		assert.Contains(t, logs.String(), "1. Yes")
		assert.Contains(t, logs.String(), "2. No")
		if environment == "" {
			assert.Contains(t, logs.String(), `"local"`)
		} else {
			assert.Contains(t, logs.String(), `"production"`)
		}
		assert.Empty(t, output.String())
	}
}

func TestCreateAndRolloutConfirmationCanBeDeclined(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := &cobra.Command{}
	cmd.SetContext(system.WithInteraction(ctx, true))
	cmd.SetIn(strings.NewReader("\r"))
	cmd.Flags().Bool("rollout", true, "")
	var output, logs bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&logs)
	backend := &deploymentFakeBackend{deployment: deployment.Deployment{Reference: "new-build"}}
	require.NoError(t, runProjectDeploymentCreate(cmd, backend, "", deployment.CreateOptions{}))
	assert.Equal(t, 1, backend.createCalls)
	assert.Zero(t, backend.rolloutCalls)
	assert.Equal(t, "Created deployment \"new-build\"\n", output.String())
}

func TestDeploymentConfirmationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	confirmed, err := confirmDeployment(cmd, deployment.Deployment{Reference: "unused"}, rolloutSelection)
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, confirmed)
}
