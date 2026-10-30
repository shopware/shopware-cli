//go:build deployment

package project

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
)

var projectDeploymentRolloutCmd = &cobra.Command{
	Use:   "rollout [deployment-reference]",
	Short: "Roll out an existing deployment without rebuilding",
	Example: `  # With ssh.directory: /var/www/shop/live, activate /var/www/shop/live
  # and store releases in /var/www/shop/releases.
  shopware-cli project deploy rollout release-name -e production`,
	Args:              deploymentSelectionArgs(rolloutSelection),
	ValidArgsFunction: deploymentReferenceCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolveDeploymentProjectRoot(nil)
		if err != nil {
			return err
		}
		target, err := resolveProjectDeploymentBackend(cmd, root)
		if err != nil {
			return err
		}
		artifact, selected, err := selectDeployment(cmd, target, args, rolloutSelection)
		if err != nil || !selected {
			return err
		}
		return runProjectDeploymentRollout(cmd, target, artifact)
	},
}

func runProjectDeploymentRollout(cmd *cobra.Command, backend deployment.Backend, artifact deployment.Deployment) error {
	environment, _ := cmd.Flags().GetString("env")
	if err := deployment.WritePreflight(cmd.Context(), backend, deployment.PreflightOptions{
		Action: deployment.PreflightRollout, Environment: environment, Deployment: artifact,
	}, cmd.ErrOrStderr()); err != nil {
		return err
	}
	confirmed, err := confirmDeployment(cmd, artifact, rolloutSelection)
	if err != nil || !confirmed {
		return err
	}
	rollout, err := backend.RolloutDeployment(cmd.Context(), artifact, cmd.ErrOrStderr())
	if err != nil {
		return fmt.Errorf("roll out deployment: %w", err)
	}
	if rollout.Reference == "" {
		return errors.New("roll out deployment: backend returned an empty rollout reference")
	}
	if rollout.Unchanged {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Deployment %q is already active; nothing to do\n", artifact.Reference)
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Deployed %q successfully\n", artifact.Reference)
	return err
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentRolloutCmd)
}
