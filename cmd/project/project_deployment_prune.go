//go:build deployment

package project

import (
	"errors"
	"fmt"
	"io"

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
increased. Removed deployments cannot be reactivated without preparing them again.

For multiple hosts, apply retention independently and preserve each host's active
deployment. Successful results are shown with host names even if another host fails.
Cleanup is not atomic across hosts; failures do not undo completed removals.`,
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
		err = fmt.Errorf("prune deployments: %w", err)
	}
	verb := "Pruned"
	if options.DryRun {
		verb = "Would prune"
	}
	if len(result.Hosts) > 0 {
		for _, host := range result.Hosts {
			hostResult := deployment.DeploymentPruneResult{Deployments: host.Deployments, Artifacts: host.Artifacts}
			if writeErr := writeDeploymentPruneResult(cmd.OutOrStdout(), hostResult, verb, "["+host.Host+"] "); writeErr != nil {
				return errors.Join(err, writeErr)
			}
		}
		return err
	}
	if err != nil && len(result.Deployments) == 0 && len(result.Artifacts) == 0 {
		return err
	}
	return errors.Join(err, writeDeploymentPruneResult(cmd.OutOrStdout(), result, verb, ""))
}

func writeDeploymentPruneResult(output io.Writer, result deployment.DeploymentPruneResult, verb, prefix string) error {
	if len(result.Deployments) == 0 && len(result.Artifacts) == 0 {
		_, err := fmt.Fprintf(output, "%sNo deployments or cached artifacts to prune.\n", prefix)
		return err
	}
	for _, deployment := range result.Deployments {
		if _, err := fmt.Fprintf(output, "%s%s deployment %q\n", prefix, verb, deployment.DisplayName()); err != nil {
			return err
		}
	}
	if len(result.Artifacts) > 0 {
		_, err := fmt.Fprintf(output, "%s%s %d cached artifacts\n", prefix, verb, len(result.Artifacts))
		return err
	}
	return nil
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentPruneCmd)
	projectDeploymentPruneCmd.Flags().Int("keep", 5, "Number of distinct recent deployments to keep (active and failed/incomplete deployments are always protected)")
	projectDeploymentPruneCmd.Flags().Bool("dry-run", false, "Preview target cleanup without deleting anything")
}
