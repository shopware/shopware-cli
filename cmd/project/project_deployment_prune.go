//go:build deployment

package project

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
)

var projectDeploymentPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old deployments on the target",
	Long: `Remove old deployments and their stored helper logs from the selected target.
Keep the most recently activated distinct successful deployments (5 by default),
plus the active deployment if it falls outside that set. Failed, incomplete and
unmanaged releases are retained.

For SSH, remove pruned release directories, their metadata and helper logs, and
cached archives that are no longer referenced by retained deployment metadata.
Shared data and local archives are never removed. Cleanup uses the same lock as
rollout and initialization. Use --dry-run to preview removals without deleting
anything. Interrupted cleanup is resumed on the next prune, even if --keep was
increased. Removed deployments cannot be reactivated without preparing them again.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		keep, err := cmd.Flags().GetInt("keep")
		if err != nil {
			return err
		}
		if keep < 0 {
			return errors.New("--keep must not be negative")
		}
		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}
		root, err := resolveDeploymentProjectRoot(nil)
		if err != nil {
			return err
		}
		target, err := resolveProjectDeploymentBackend(cmd, root)
		if err != nil {
			return err
		}
		return runProjectDeploymentPrune(cmd, target, deployment.DeploymentPruneOptions{Keep: keep, DryRun: dryRun})
	},
}

func runProjectDeploymentPrune(cmd *cobra.Command, target deployment.Backend, options deployment.DeploymentPruneOptions) error {
	backend, ok := target.(deployment.DeploymentPruner)
	if !ok {
		return fmt.Errorf("pruning deployments with backend %q: %w", target.Type(), deployment.ErrNotSupported)
	}
	result, err := backend.PruneDeployments(cmd.Context(), options)
	if err != nil {
		return fmt.Errorf("prune deployments: %w", err)
	}
	if len(result.Deployments) == 0 && len(result.Artifacts) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No deployments or cached artifacts to prune.")
		return err
	}
	verb := "Pruned"
	if options.DryRun {
		verb = "Would prune"
	}
	for _, deployment := range result.Deployments {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s deployment %q\n", verb, deployment.DisplayName()); err != nil {
			return err
		}
	}
	if len(result.Artifacts) > 0 {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %d cached artifacts\n", verb, len(result.Artifacts))
		return err
	}
	return nil
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentPruneCmd)
	projectDeploymentPruneCmd.Flags().Int("keep", 5, "Number of distinct recent deployments to keep (active and failed/incomplete deployments are always protected)")
	projectDeploymentPruneCmd.Flags().Bool("dry-run", false, "Preview target cleanup without deleting anything")
}
