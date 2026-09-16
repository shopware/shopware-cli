//go:build deployment

package project

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/projectbuild"
)

var projectDeploymentRolloutCmd = &cobra.Command{
	Use:   "rollout <deployment-reference>",
	Short: "Roll out an existing deployment without rebuilding",
	Long: `Roll out an existing deployment to the selected environment.
For SSH, the reference is a local tar.gz archive and ssh.directory must point
to an absolute current path, for example /var/www/shop/current. Releases and
shared data live in sibling releases/ and shared/ directories.

By default, provision shared/.env.local (or a remote DATABASE_URL) before rollout.
Configure persistent paths with environments.<name>.ssh.shared.files and
shared.directories. Omitted lists use Shopware defaults; explicit lists replace
them, and [] disables sharing for that category. Paths must be project-relative
and must not overlap. The helper validates the resulting runtime configuration.
The remote host needs PHP CLI and tar; the archive must contain the deployment
helper and bin/console. A deployment lock covers upload, preparation, and the
atomic current symlink switch. Helper output goes to stderr; the successful
rollout reference goes to stdout. Old and failed releases are retained.

The deployment helper and pre-rollout system checks run before activation.
Migrations may change the shared database even when preparation fails. This is
not a database rollback mechanism. Configure web serving through current/public;
OPcache, long-running workers, and backward-compatible migrations still need
an application-specific strategy for zero downtime.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolveDeploymentProjectRoot(nil)
		if err != nil {
			return err
		}
		target, err := resolveProjectDeploymentExecutor(cmd, root, projectbuild.ArchiveOptions{})
		if err != nil {
			return err
		}
		return runProjectDeploymentRollout(cmd, target, executor.Deployment{Reference: args[0]})
	},
}

func runProjectDeploymentRollout(cmd *cobra.Command, target executor.Executor, deployment executor.Deployment) error {
	backend, ok := target.(executor.DeploymentExecutor)
	if !ok {
		return fmt.Errorf("rolling out deployments with executor %q: %w", target.Type(), executor.ErrNotSupported)
	}
	rollout, err := backend.RolloutDeployment(cmd.Context(), deployment)
	if err != nil {
		return fmt.Errorf("roll out deployment: %w", err)
	}
	if rollout.Reference == "" {
		return errors.New("roll out deployment: backend returned an empty rollout reference")
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), rollout.Reference)
	return err
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentRolloutCmd)
}
