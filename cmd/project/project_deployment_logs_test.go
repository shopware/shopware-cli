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
)

type deploymentLogsFakeBackend struct {
	deploymentTestBackend
	received deployment.Deployment
	logs     string
	err      error
}

func (f *deploymentLogsFakeBackend) WriteDeploymentLogs(_ context.Context, artifact deployment.Deployment, output io.Writer) error {
	f.received = artifact
	if f.err != nil {
		return f.err
	}
	_, err := io.WriteString(output, f.logs)
	return err
}

func TestProjectDeploymentLogs(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	var output bytes.Buffer
	cmd.SetOut(&output)
	artifact := deployment.Deployment{Reference: "happy-euclid"}
	target := &deploymentLogsFakeBackend{logs: "helper stdout\nhelper stderr\n"}
	require.NoError(t, runProjectDeploymentLogs(cmd, target, artifact))
	assert.Equal(t, artifact, target.received)
	assert.Equal(t, target.logs, output.String())

	target.err = context.Canceled
	require.ErrorIs(t, runProjectDeploymentLogs(cmd, target, artifact), context.Canceled)
	target.err = errors.New("logs not found")
	require.ErrorContains(t, runProjectDeploymentLogs(cmd, target, artifact), "read deployment logs: logs not found")
	require.ErrorIs(t, runProjectDeploymentLogs(cmd, deploymentTestBackend{}, artifact), deployment.ErrNotSupported)

	target.err = nil
	writeErr := errors.New("output closed")
	cmd.SetOut(deploymentErrorWriter{err: writeErr})
	require.ErrorIs(t, runProjectDeploymentLogs(cmd, target, artifact), writeErr)
}
