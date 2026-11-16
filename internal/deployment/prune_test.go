package deployment

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pruneBackend struct {
	calls  []DeploymentPruneOptions
	result DeploymentPruneResult
	err    error
}

func (b *pruneBackend) PruneDeployments(_ context.Context, options DeploymentPruneOptions) (DeploymentPruneResult, error) {
	b.calls = append(b.calls, options)
	return b.result, b.err
}

func TestPruneConfirmation(t *testing.T) {
	failure := errors.New("failed")
	for _, tc := range []struct {
		name                                    string
		confirm, dryRun, nonInteractive, cancel bool
		previewErr, confirmErr                  error
		wantCalls                               int
		wantResult                              bool
	}{
		{name: "confirmed", confirm: true, wantCalls: 2, wantResult: true},
		{name: "declined", wantCalls: 1},
		{name: "confirmation failed", confirmErr: failure, wantCalls: 1},
		{name: "preview failed", previewErr: failure, wantCalls: 1},
		{name: "cancelled after preview", confirm: true, cancel: true, wantCalls: 1},
		{name: "dry run", dryRun: true, wantCalls: 1, wantResult: true},
		{name: "non-interactive", nonInteractive: true, wantCalls: 1, wantResult: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			backend := &pruneBackend{result: DeploymentPruneResult{
				Hosts: []DeploymentPruneHostResult{{Host: "production", Deployments: []Deployment{{Reference: "old-release"}}, Artifacts: []string{"old-archive"}}},
			}, err: tc.previewErr}
			prompted := false
			confirm := func(preview DeploymentPruneResult) (bool, error) {
				prompted = true
				assert.Equal(t, backend.result, preview)
				require.Equal(t, []DeploymentPruneOptions{{Keep: 1, DryRun: true}}, backend.calls)
				if tc.cancel {
					cancel()
				}
				return tc.confirm, tc.confirmErr
			}
			if tc.nonInteractive {
				confirm = nil
			}
			result, err := Prune(ctx, backend, DeploymentPruneOptions{Keep: 1, DryRun: tc.dryRun}, confirm)
			if tc.cancel {
				require.ErrorIs(t, err, context.Canceled)
			} else if tc.previewErr != nil || tc.confirmErr != nil {
				require.ErrorIs(t, err, failure)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, backend.calls, tc.wantCalls)
			assert.Equal(t, tc.wantResult, result != nil)
			assert.Equal(t, !tc.nonInteractive && !tc.dryRun && tc.previewErr == nil, prompted)
			if tc.wantResult {
				assert.Equal(t, DeploymentPruneOptions{Keep: 1, DryRun: tc.dryRun}, backend.calls[len(backend.calls)-1])
			}
		})
	}
}
