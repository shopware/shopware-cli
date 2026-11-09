//go:build deployment

package project

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
)

var projectDeploymentRollbackCmd = &cobra.Command{
	Use:   "rollback [deployment-reference]",
	Short: "Reactivate a deployment already on the target",
	Long: `Reactivate a deployment retained on the selected target without rebuilding it.
Provide its deployment reference, or omit it to choose from retained deployments
interactively. Use -e to select the environment.

The command runs preflight checks and asks for confirmation before activation.
In non-interactive mode, a reference is required and confirmation is skipped.`,
	Args:              deploymentSelectionArgs(rollbackSelection),
	ValidArgsFunction: rollbackReferenceCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolveDeploymentProjectRoot(nil)
		if err != nil {
			return err
		}
		backend, err := resolveProjectDeploymentBackend(cmd, root)
		if err != nil {
			return err
		}
		if _, ok := backend.(deployment.DeploymentActivator); !ok {
			return fmt.Errorf("activating deployments with backend %q: %w", backend.Type(), deployment.ErrNotSupported)
		}
		artifact, selected, err := selectDeployment(cmd, backend, args, rollbackSelection)
		if err != nil || !selected {
			return err
		}
		return runProjectDeploymentRollback(cmd, backend, artifact)
	},
}

func runProjectDeploymentRollback(cmd *cobra.Command, backend deployment.Backend, artifact deployment.Deployment) error {
	activator, ok := backend.(deployment.DeploymentActivator)
	if !ok {
		return fmt.Errorf("activating deployments with backend %q: %w", backend.Type(), deployment.ErrNotSupported)
	}
	environment, _ := cmd.Flags().GetString("env")
	if err := deployment.WritePreflight(cmd.Context(), backend, deployment.PreflightOptions{
		Action: deployment.PreflightRollback, Environment: environment, Deployment: artifact,
	}, cmd.ErrOrStderr()); err != nil {
		return err
	}
	confirmed, err := confirmDeployment(cmd, artifact, rollbackSelection)
	if err != nil || !confirmed {
		return err
	}
	rollout, err := activator.ActivateDeployment(cmd.Context(), artifact, cmd.ErrOrStderr())
	if err != nil {
		return fmt.Errorf("roll back deployment: %w", err)
	}
	if rollout.Reference == "" {
		return errors.New("roll back deployment: backend returned an empty rollout reference")
	}
	if rollout.Unchanged {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Deployment %q is already active; nothing to do\n", artifact.DisplayName())
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Reactivated deployment %q successfully\n", artifact.DisplayName())
	return err
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentRollbackCmd)
}
