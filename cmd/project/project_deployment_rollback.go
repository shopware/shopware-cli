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
	Long: `Reactivate a retained successful deployment on the selected target.
Without a reference, choose from an interactive picker showing remote deployments
and their creation time, newest first. Non-interactive use requires an explicit reference.
Shell completion uses the same remote candidates.
Interactive sessions ask for confirmation before activation. Use --no-interaction
to skip the prompt in automation.

For SSH, the reference is a remote deployment name. The local archive and remote
archive cache are not required. This only switches current to an existing prepared
release and records a new activation. It does not build, upload, extract, run
Deployment Helper, replace helper logs, or roll back the database.
An already-active deployment is a no-op. Missing, failed, incomplete, pruning,
or inconsistent releases cannot be activated.
If environments.<name>.ssh.cachetool is enabled, OPcache is reset after activation.
A failed reset warns without undoing the activation.
On targets whose hostname -f contains de-nserver.de, unset/disabled CacheTool instead uses
SIGTERM on other PHP processes owned by the SSH user (never the root account).
Other shops and workers sharing that account are affected.

The application code must remain compatible with the current database schema.`,
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
