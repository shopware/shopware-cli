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
	"github.com/shopware/shopware-cli/internal/system"
)

type deploymentCandidateFakeBackend struct {
	deploymentTestBackend
	local, remote           []deployment.Candidate
	localCalls, remoteCalls int
	err                     error
}

func (f *deploymentCandidateFakeBackend) RolloutCandidates(context.Context) ([]deployment.Candidate, error) {
	f.localCalls++
	return f.local, f.err
}

func (f *deploymentCandidateFakeBackend) RollbackCandidates(context.Context) ([]deployment.Candidate, error) {
	f.remoteCalls++
	return f.remote, f.err
}

func TestDeploymentCandidateSources(t *testing.T) {
	backend := &deploymentCandidateFakeBackend{
		local:  []deployment.Candidate{{Deployment: deployment.Deployment{Reference: "local-build"}}},
		remote: []deployment.Candidate{{Deployment: deployment.Deployment{Reference: "remote-build"}}},
	}
	local, err := deploymentSelectionCandidates(t.Context(), backend, rolloutSelection)
	require.NoError(t, err)
	assert.Equal(t, backend.local, local)
	remote, err := deploymentSelectionCandidates(t.Context(), backend, rollbackSelection)
	require.NoError(t, err)
	assert.Equal(t, backend.remote, remote)
	assert.Equal(t, 1, backend.localCalls)
	assert.Equal(t, 1, backend.remoteCalls)
}

func TestDeploymentSelectionExplicitReference(t *testing.T) {
	for _, kind := range []deploymentSelectionKind{rolloutSelection, rollbackSelection} {
		cmd := &cobra.Command{}
		cmd.SetContext(system.WithInteraction(t.Context(), false))
		for _, reference := range []string{"opaque-build-id", "./custom/build.tar.gz"} {
			// Explicit references need neither discovery capability nor a picker.
			artifact, selected, err := selectDeployment(cmd, deploymentTestBackend{}, []string{reference}, kind)
			require.NoError(t, err)
			assert.True(t, selected)
			assert.Equal(t, reference, artifact.Reference)
		}
	}
}

func TestDeploymentSelectionRequiresInteraction(t *testing.T) {
	for _, kind := range []deploymentSelectionKind{rolloutSelection, rollbackSelection} {
		cmd := &cobra.Command{}
		cmd.SetContext(system.WithInteraction(t.Context(), false))
		backend := &deploymentCandidateFakeBackend{}
		_, selected, err := selectDeployment(cmd, backend, nil, kind)
		require.ErrorContains(t, err, "requires a deployment reference in non-interactive mode")
		assert.False(t, selected)
		assert.Zero(t, backend.localCalls)
		assert.Zero(t, backend.remoteCalls)
	}
}

func TestDeploymentSelectionErrors(t *testing.T) {
	for _, kind := range []deploymentSelectionKind{rolloutSelection, rollbackSelection} {
		cmd := &cobra.Command{}
		cmd.SetContext(system.WithInteraction(t.Context(), true))
		backend := &deploymentCandidateFakeBackend{}
		_, selected, err := selectDeployment(cmd, backend, nil, kind)
		require.Error(t, err)
		assert.Contains(t, err.Error(), string(kind))
		assert.False(t, selected)
		backend.err = errors.New("discovery failed")
		_, selected, err = selectDeployment(cmd, backend, nil, kind)
		require.ErrorIs(t, err, backend.err)
		assert.False(t, selected)
		_, _, err = selectDeployment(cmd, deploymentTestBackend{}, nil, kind)
		require.ErrorIs(t, err, deployment.ErrNotSupported)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		cmd.SetContext(ctx)
		_, selected, err = selectDeployment(cmd, backend, nil, kind)
		require.ErrorIs(t, err, context.Canceled)
		assert.False(t, selected)
	}
}

func TestDeploymentPicker(t *testing.T) {
	for _, kind := range []deploymentSelectionKind{rolloutSelection, rollbackSelection} {
		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())
		cmd.SetIn(strings.NewReader("2\n"))
		var stdout, stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		activated := time.Now().Add(-time.Minute)
		built := activated.Add(-4 * time.Minute)
		created := activated.Add(-time.Hour)
		candidates := []deployment.Candidate{
			{Deployment: deployment.Deployment{Reference: "current-id", Name: "Current build"}, Active: true, BuiltAt: &built, CreatedAt: &created, DeployedAt: &activated},
			{Deployment: deployment.Deployment{Reference: "older-id", Name: "Previous build"}},
		}
		var selected deployment.Deployment
		form := deploymentPickerForm(cmd, candidates, kind, &selected).WithAccessible(true)
		require.NoError(t, form.RunWithContext(t.Context()))
		assert.Equal(t, candidates[1].Deployment, selected)
		assert.Contains(t, stderr.String(), "Current build")
		assert.Contains(t, stderr.String(), "(active)")
		if kind == rolloutSelection {
			assert.Contains(t, stderr.String(), "built 5 minutes ago")
		} else {
			assert.Contains(t, stderr.String(), "created 1 hour ago")
		}
		assert.NotContains(t, stderr.String(), "UTC")
		assert.Empty(t, stdout.String(), "picker output must not pollute command results")
	}
}

func TestDeploymentCandidateRelativeAges(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	built := now.Add(-5 * time.Minute)
	created := now.Add(-48 * time.Hour)
	candidate := deployment.Candidate{
		Deployment: deployment.Deployment{Reference: "build-id"},
		BuiltAt:    &built, CreatedAt: &created, DeployedAt: &now, Active: true,
	}
	assert.Equal(t, "build-id — built 5 minutes ago (active)", deploymentCandidateLabel(candidate, rolloutSelection, now))
	assert.Equal(t, "build-id — created 2 days ago (active)", deploymentCandidateLabel(candidate, rollbackSelection, now))
	candidate.DeployedAt = new(now.Add(-time.Hour))
	assert.Equal(t, "build-id — created 2 days ago (active)", deploymentCandidateLabel(candidate, rollbackSelection, now))
	candidate.Active = false
	candidate.BuiltAt = new(now.Add(5 * time.Minute))
	assert.Equal(t, "build-id — built 5 minutes from now", deploymentCandidateLabel(candidate, rolloutSelection, now))
	candidate.BuiltAt = &now
	assert.Equal(t, "build-id — built now", deploymentCandidateLabel(candidate, rolloutSelection, now))
	candidate.BuiltAt = nil
	assert.Equal(t, "build-id", deploymentCandidateLabel(candidate, rolloutSelection, now))
	candidate.CreatedAt = new(time.Time{})
	assert.Equal(t, "build-id", deploymentCandidateLabel(candidate, rollbackSelection, now))
}

func TestDeploymentPickerPreselectsNewestBuild(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	cmd.SetIn(strings.NewReader("\n"))
	cmd.SetErr(new(bytes.Buffer))
	candidates := []deployment.Candidate{
		{Deployment: deployment.Deployment{Reference: "newest-build"}},
		{Deployment: deployment.Deployment{Reference: "older-build"}},
	}
	var selected deployment.Deployment
	form := deploymentPickerForm(cmd, candidates, rolloutSelection, &selected).WithAccessible(true)
	assert.Equal(t, candidates[0].Deployment, selected)
	require.NoError(t, form.RunWithContext(t.Context()))
	assert.Equal(t, candidates[0].Deployment, selected, "Enter must accept the first/newest candidate")
}

func TestDeploymentPickerCancellation(t *testing.T) {
	for _, kind := range []deploymentSelectionKind{rolloutSelection, rollbackSelection} {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		cmd := &cobra.Command{}
		cmd.SetContext(ctx)
		cmd.SetIn(strings.NewReader("\x03"))
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(new(bytes.Buffer))
		candidates := []deployment.Candidate{{Deployment: deployment.Deployment{Reference: "unused"}}}
		backend := &deploymentCandidateFakeBackend{local: candidates, remote: candidates}
		selected, ok, err := selectDeployment(cmd, backend, nil, kind)
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Empty(t, selected.Reference)
	}
}
