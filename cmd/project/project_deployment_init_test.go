//go:build deployment

package project

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
)

type deploymentInitFakeBackend struct {
	deploymentTestBackend
	called   bool
	err      error
	guided   bool
	guideErr error
}

func (f *deploymentInitFakeBackend) WriteInitializationNextSteps(_ string, _ io.Writer) error {
	f.guided = true
	return f.guideErr
}

func (f *deploymentInitFakeBackend) InitializeDeployment(context.Context, deployment.InitializationOptions) error {
	f.called = true
	return f.err
}

func TestRunProjectDeploymentInit(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	fake := &deploymentInitFakeBackend{}
	require.NoError(t, runProjectDeploymentInit(cmd, fake))
	assert.True(t, fake.called)
	assert.True(t, fake.guided)

	expected := errors.New("initialization failed")
	fake = &deploymentInitFakeBackend{err: expected}
	require.ErrorIs(t, runProjectDeploymentInit(cmd, fake), expected)
	assert.True(t, fake.called)
	assert.False(t, fake.guided)

	fake = &deploymentInitFakeBackend{err: deployment.ErrInitializationCanceled}
	require.NoError(t, runProjectDeploymentInit(cmd, fake))
	assert.True(t, fake.called)
	assert.False(t, fake.guided)

	fake = &deploymentInitFakeBackend{guideErr: io.ErrClosedPipe}
	require.ErrorIs(t, runProjectDeploymentInit(cmd, fake), io.ErrClosedPipe)
	assert.True(t, fake.guided)

	require.ErrorIs(t, runProjectDeploymentInit(cmd, deploymentTestBackend{}), deployment.ErrNotSupported)
}
