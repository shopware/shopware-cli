//go:build deployment

package project

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/system"
)

type deploymentInitFakeBackend struct {
	deploymentTestBackend
	called bool
	err    error
}

func (f *deploymentInitFakeBackend) InitializeDeployment(context.Context) error {
	f.called = true
	return f.err
}

func TestRunProjectDeploymentInit(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	fake := &deploymentInitFakeBackend{}
	require.NoError(t, runProjectDeploymentInit(cmd, fake))
	assert.True(t, fake.called)

	expected := errors.New("initialization failed")
	fake = &deploymentInitFakeBackend{err: expected}
	require.ErrorIs(t, runProjectDeploymentInit(cmd, fake), expected)
	assert.True(t, fake.called)

	require.ErrorIs(t, runProjectDeploymentInit(cmd, deploymentTestBackend{}), deployment.ErrNotSupported)
}

func TestProjectDeploymentInitRequiresInteraction(t *testing.T) {
	cmd, out := newLifecycleCommand(t, []string{"init"})
	err := cmd.ExecuteContext(system.WithInteraction(t.Context(), false))
	require.ErrorContains(t, err, "requires interaction")
	assert.Empty(t, out.String())
}
