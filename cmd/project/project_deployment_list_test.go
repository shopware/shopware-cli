//go:build deployment

package project

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/tui"
)

type rolloutHistoryFakeBackend struct {
	deploymentTestBackend
	rollouts []deployment.Rollout
	err      error
}

func (f *rolloutHistoryFakeBackend) ListRollouts(context.Context) ([]deployment.Rollout, error) {
	return f.rollouts, f.err
}

func TestProjectDeploymentList(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	var output bytes.Buffer
	cmd.SetOut(&output)
	deployedAt := time.Date(2026, 9, 22, 11, 29, 12, 123000000, time.UTC)
	target := &rolloutHistoryFakeBackend{rollouts: []deployment.Rollout{
		{Reference: "release-new", Deployment: deployment.Deployment{Reference: "focused-turing"}, Active: true, DeployedAt: &deployedAt},
		{Reference: "release-old", Deployment: deployment.Deployment{Reference: "clever-hopper"}},
		{Reference: "release-first", Deployment: deployment.Deployment{Reference: "focused-turing"}},
	}}

	require.NoError(t, runProjectDeploymentList(cmd, target, tui.TableFormatJSON))
	assert.JSONEq(t, `[
		{"release":"release-new","deployment":"focused-turing","active":true,"deployed_at":"2026-09-22T11:29:12.123Z"},
		{"release":"release-old","deployment":"clever-hopper","active":false,"deployed_at":null},
		{"release":"release-first","deployment":"focused-turing","active":false,"deployed_at":null}
	]`, output.String())

	output.Reset()
	require.NoError(t, runProjectDeploymentList(cmd, target, tui.TableFormatTable))
	assert.Contains(t, output.String(), "Status")
	assert.Contains(t, output.String(), "Deployed at (UTC)")
	assert.Regexp(t, `focused-turing[^\n]*Active[^\n]*2026-09-22 11:29:12`, output.String())
	assert.Regexp(t, `clever-hopper[^\n]*Inactive[^\n]*—`, output.String())
	assert.Equal(t, 2, strings.Count(output.String(), "focused-turing"), "repeated rollouts must remain separate rows")
	assert.NotContains(t, output.String(), "Release")
	assert.NotContains(t, output.String(), "release-")
	assert.NotContains(t, output.String(), "true")
	assert.NotContains(t, output.String(), "false")

	target.err = errors.New("remote unavailable")
	require.ErrorContains(t, runProjectDeploymentList(cmd, target, tui.TableFormatTable), "list deployments: remote unavailable")
	require.ErrorIs(t, runProjectDeploymentList(cmd, deploymentTestBackend{}, tui.TableFormatTable), deployment.ErrNotSupported)
}

func TestProjectDeploymentListDisplayNamePreservesJSONReference(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	var output bytes.Buffer
	cmd.SetOut(&output)
	target := &rolloutHistoryFakeBackend{rollouts: []deployment.Rollout{{
		Reference: "release-id",
		Deployment: deployment.Deployment{
			Reference: "/Users/shyim/Downloads/my-fancy-shop/.shopware-cli/deployments/happy-euclid.tar.gz",
			Name:      "happy-euclid",
		},
	}}}
	require.NoError(t, runProjectDeploymentList(cmd, target, tui.TableFormatTable))
	assert.Contains(t, output.String(), "happy-euclid")
	assert.NotContains(t, output.String(), "/Users/")
	assert.NotContains(t, output.String(), ".tar.gz")

	output.Reset()
	require.NoError(t, runProjectDeploymentList(cmd, target, tui.TableFormatJSON))
	assert.JSONEq(t, `[{
		"release":"release-id",
		"deployment":"/Users/shyim/Downloads/my-fancy-shop/.shopware-cli/deployments/happy-euclid.tar.gz",
		"active":false,
		"deployed_at":null
	}]`, output.String())
}
