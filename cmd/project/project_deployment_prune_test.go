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

func TestProjectDeploymentPruneHostResults(t *testing.T) {
	hostErr := errors.New("host web-3: cleanup state unknown")
	for _, tc := range []struct {
		name   string
		dryRun bool
		err    error
	}{
		{name: "pruned"},
		{name: "dry run", dryRun: true},
		{name: "partial failure", err: hostErr},
		{name: "partial dry run", dryRun: true, err: hostErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetContext(t.Context())
			var output bytes.Buffer
			cmd.SetOut(&output)
			target := &deploymentPruneFakeBackend{
				result: deployment.DeploymentPruneResult{Hosts: []deployment.DeploymentPruneHostResult{
					{Host: "web-1", Deployments: []deployment.Deployment{{Reference: "happy-euclid"}}, Artifacts: []string{"archive"}},
					{Host: "web-2"},
				}},
				err: tc.err,
			}
			options := deployment.DeploymentPruneOptions{Keep: 3, DryRun: tc.dryRun}
			err := runProjectDeploymentPrune(cmd, target, options)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, options, target.options)
			verb := "Pruned"
			if tc.dryRun {
				verb = "Would prune"
			}
			assert.Equal(t, "[web-1] "+verb+" deployment \"happy-euclid\"\n"+
				"[web-1] "+verb+" 1 cached artifacts\n"+
				"[web-2] No deployments or cached artifacts to prune.\n", output.String())

			writeErr := errors.New("output closed")
			cmd.SetOut(deploymentErrorWriter{err: writeErr})
			err = runProjectDeploymentPrune(cmd, target, options)
			require.ErrorIs(t, err, writeErr)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			}
		})
	}
}

func TestProjectDeploymentPruneDefaults(t *testing.T) {
	keep, err := projectDeploymentPruneCmd.Flags().GetInt("keep")
	require.NoError(t, err)
	assert.Equal(t, 5, keep)
	dryRun, err := projectDeploymentPruneCmd.Flags().GetBool("dry-run")
	require.NoError(t, err)
	assert.False(t, dryRun)
}
