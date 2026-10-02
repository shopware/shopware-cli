//go:build deployment

package project

import (
	"context"
	"errors"
	"fmt"
	"time"

	"charm.land/huh/v2"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/system"
)

type deploymentSelectionKind string

const (
	rolloutSelection  deploymentSelectionKind = "rollout"
	rollbackSelection deploymentSelectionKind = "rollback"
)

func deploymentSelectionArgs(kind deploymentSelectionKind) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
			return err
		}
		if len(args) == 0 && !system.IsInteractionEnabled(cmd.Context()) {
			return fmt.Errorf("project deploy %s requires a deployment reference in non-interactive mode", kind)
		}
		return nil
	}
}

func deploymentSelectionCandidates(ctx context.Context, backend deployment.Backend, kind deploymentSelectionKind) ([]deployment.Candidate, error) {
	provider, ok := backend.(deployment.CandidateProvider)
	if !ok {
		return nil, fmt.Errorf("selecting deployments with backend %q: %w", backend.Type(), deployment.ErrNotSupported)
	}
	if kind == rollbackSelection {
		return provider.RollbackCandidates(ctx)
	}
	return provider.RolloutCandidates(ctx)
}

func selectDeployment(cmd *cobra.Command, backend deployment.Backend, args []string, kind deploymentSelectionKind) (deployment.Deployment, bool, error) {
	if err := deploymentSelectionArgs(kind)(cmd, args); err != nil {
		return deployment.Deployment{}, false, err
	}
	if len(args) == 1 {
		return deployment.Deployment{Reference: args[0]}, true, nil
	}
	if err := cmd.Context().Err(); err != nil {
		return deployment.Deployment{}, false, err
	}
	candidates, err := deploymentSelectionCandidates(cmd.Context(), backend, kind)
	if err != nil {
		return deployment.Deployment{}, false, fmt.Errorf("list deployments for %s: %w", kind, err)
	}
	if len(candidates) == 0 {
		if kind == rollbackSelection {
			return deployment.Deployment{}, false, errors.New("no retained deployments available for rollback on the selected target")
		}
		return deployment.Deployment{}, false, errors.New("no deployments available for rollout; create one with project deploy create")
	}
	var selected deployment.Deployment
	err = deploymentPickerForm(cmd, candidates, kind, &selected).RunWithContext(cmd.Context())
	if cmd.Context().Err() != nil {
		return deployment.Deployment{}, false, cmd.Context().Err()
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return deployment.Deployment{}, false, nil
	}
	if err != nil {
		return deployment.Deployment{}, false, err
	}
	return selected, true, nil
}

func deploymentPickerForm(cmd *cobra.Command, candidates []deployment.Candidate, kind deploymentSelectionKind, selected *deployment.Deployment) *huh.Form {
	options := make([]huh.Option[deployment.Deployment], 0, len(candidates))
	now := time.Now()
	for _, candidate := range candidates {
		label := deploymentCandidateLabel(candidate, kind, now)
		options = append(options, huh.NewOption(label, candidate.Deployment))
	}
	title := "Choose a deployment to roll out"
	if kind == rollbackSelection {
		title = "Choose a remote deployment to reactivate"
	}
	if len(candidates) > 0 {
		*selected = candidates[0].Deployment
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[deployment.Deployment]().Title(title).Options(options...).Value(selected),
	)).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr())
}

func deploymentCandidateLabel(candidate deployment.Candidate, kind deploymentSelectionKind, now time.Time) string {
	label := candidate.Deployment.DisplayName()
	timestamp, prefix := candidate.BuiltAt, "built"
	if kind == rollbackSelection {
		timestamp, prefix = candidate.CreatedAt, "created"
	}
	if timestamp != nil && !timestamp.IsZero() {
		label += " — " + prefix + " " + humanize.RelTime(*timestamp, now, "ago", "from now")
	}
	if candidate.Active {
		label += " (active)"
	}
	return label
}
