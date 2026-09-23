//go:build deployment

package project

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
)

type deploymentPruneFakeBackend struct {
	deploymentTestBackend
	options deployment.DeploymentPruneOptions
	result  deployment.DeploymentPruneResult
	err     error
}

func (f *deploymentPruneFakeBackend) PruneDeployments(_ context.Context, options deployment.DeploymentPruneOptions) (deployment.DeploymentPruneResult, error) {
	f.options = options
	return f.result, f.err
}

func TestProjectDeploymentPrune(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())
		var output bytes.Buffer
		cmd.SetOut(&output)
		target := &deploymentPruneFakeBackend{result: deployment.DeploymentPruneResult{
			Deployments: []deployment.Deployment{{Reference: "happy-euclid"}, {Reference: "focused-turing"}},
			Artifacts:   []string{"archive-a", "archive-b"},
		}}
		options := deployment.DeploymentPruneOptions{Keep: 5, DryRun: dryRun}
		require.NoError(t, runProjectDeploymentPrune(cmd, target, options))
		assert.Equal(t, options, target.options)
		verb := "Pruned"
		if dryRun {
			verb = "Would prune"
		}
		assert.Equal(t, verb+" deployment \"happy-euclid\"\n"+verb+" deployment \"focused-turing\"\n"+verb+" 2 cached artifacts\n", output.String())
		output.Reset()
		target.result = deployment.DeploymentPruneResult{}
		require.NoError(t, runProjectDeploymentPrune(cmd, target, options))
		assert.Equal(t, "No deployments or cached artifacts to prune.\n", output.String())
	}
}

func TestProjectDeploymentPruneErrors(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	var output bytes.Buffer
	cmd.SetOut(&output)
	options := deployment.DeploymentPruneOptions{Keep: 5}
	target := &deploymentPruneFakeBackend{err: context.Canceled}
	require.ErrorIs(t, runProjectDeploymentPrune(cmd, target, options), context.Canceled)
	assert.Empty(t, output.String())
	require.ErrorIs(t, runProjectDeploymentPrune(cmd, deploymentTestBackend{}, options), deployment.ErrNotSupported)
	writeErr := errors.New("output closed")
	target.err = nil
	cmd.SetOut(deploymentErrorWriter{err: writeErr})
	require.ErrorIs(t, runProjectDeploymentPrune(cmd, target, options), writeErr)
}

func TestProjectDeploymentPruneDefaults(t *testing.T) {
	keep, err := projectDeploymentPruneCmd.Flags().GetInt("keep")
	require.NoError(t, err)
	assert.Equal(t, 5, keep)
	dryRun, err := projectDeploymentPruneCmd.Flags().GetBool("dry-run")
	require.NoError(t, err)
	assert.False(t, dryRun)
}
