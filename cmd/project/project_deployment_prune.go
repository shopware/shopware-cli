//go:build deployment

package project

import (
	"errors"
	"fmt"
	"io"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectDeploymentPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old deployments on the target",
	Long: `Remove old SSH deployments and unreferenced cached artifacts from the target.
Keep the most recent distinct deployments according to --keep. Active, failed,
and incomplete deployments are protected; interrupted cleanup is resumed.
Related runtime data, metadata, and logs are removed with each deployment.
Artifact cleanup is skipped when archive references cannot be determined safely.
Interactive runs preview cleanup and ask for confirmation before deleting.
In non-interactive mode, confirmation is skipped.
Use --dry-run to preview cleanup without changing the target.`,
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
	var confirm func(deployment.DeploymentPruneResult) (bool, error)
	if system.IsInteractionEnabled(cmd.Context()) {
		confirm = func(preview deployment.DeploymentPruneResult) (bool, error) {
			return confirmProjectDeploymentPrune(cmd, preview)
		}
	}
	result, err := deployment.Prune(cmd.Context(), backend, options, confirm)
	if err != nil {
		err = fmt.Errorf("cannot prune deployments: %w", err)
	}
	if result == nil {
		return err
	}
	verb := "Pruned"
	if options.DryRun {
		verb = "Would prune"
	}
	return writeProjectDeploymentPruneResult(cmd.OutOrStdout(), *result, verb, err)
}

func confirmProjectDeploymentPrune(cmd *cobra.Command, preview deployment.DeploymentPruneResult) (bool, error) {
	if err := writeProjectDeploymentPruneResult(cmd.ErrOrStderr(), preview, "Would prune", nil); err != nil {
		return false, err
	}
	environment, _ := cmd.Flags().GetString("env")
	if environment == "" {
		environment = "local"
	}
	confirmed := false
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[bool]().
			Title(fmt.Sprintf("Prune deployments and cached artifacts on environment %q?", environment)).
			Options(huh.NewOption("Yes", true), huh.NewOption("No", false)).
			Value(&confirmed),
	)).WithTheme(tui.ShopwareTheme()).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr()).RunWithContext(cmd.Context())
	if cmd.Context().Err() != nil {
		return false, cmd.Context().Err()
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return false, nil
	}
	return confirmed && err == nil, err
}

func writeProjectDeploymentPruneResult(output io.Writer, result deployment.DeploymentPruneResult, verb string, err error) error {
	if len(result.Hosts) > 0 {
		for _, host := range result.Hosts {
			hostResult := deployment.DeploymentPruneResult{Deployments: host.Deployments, Artifacts: host.Artifacts, ArtifactSkipReasons: host.ArtifactSkipReasons, IncompleteDeployments: host.IncompleteDeployments}
			if writeErr := writeDeploymentPruneResult(output, hostResult, verb, "["+host.Host+"] "); writeErr != nil {
				return errors.Join(err, writeErr)
			}
		}
		return err
	}
	if err != nil && len(result.Deployments) == 0 && len(result.Artifacts) == 0 && len(result.ArtifactSkipReasons) == 0 && len(result.IncompleteDeployments) == 0 {
		return err
	}
	return errors.Join(err, writeDeploymentPruneResult(output, result, verb, ""))
}

func writeDeploymentPruneResult(output io.Writer, result deployment.DeploymentPruneResult, verb, prefix string) error {
	for _, reason := range result.ArtifactSkipReasons {
		if _, err := fmt.Fprintf(output, "%sSkipped cached artifact cleanup: %s\n", prefix, reason); err != nil {
			return err
		}
	}
	for _, name := range result.IncompleteDeployments {
		if _, err := fmt.Fprintf(output, "%sCleanup incomplete for deployment %q; run prune again to resume.\n", prefix, name); err != nil {
			return err
		}
	}
	if len(result.Deployments) == 0 && len(result.Artifacts) == 0 && len(result.ArtifactSkipReasons) == 0 && len(result.IncompleteDeployments) == 0 {
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
