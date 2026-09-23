//go:build deployment

package project

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
)

var projectDeploymentRolloutCmd = &cobra.Command{
	Use:   "rollout <deployment-reference>",
	Short: "Roll out an existing deployment without rebuilding",
	Long: `Roll out an existing deployment to the selected environment.
For SSH, the reference may be a generated deployment name from
.shopware-cli/deployments or a local tar.gz path. Generated names support shell
completion. ssh.directory must point to an absolute current path, for example
/var/www/shop/current. Releases and shared data live in sibling releases/ and
shared/ directories.

By default, provision shared/.env.local (or a remote DATABASE_URL) before rollout.
Configure persistent paths with environments.<name>.ssh.shared.files and
shared.directories. Omitted lists use Shopware defaults; explicit lists replace
them, and [] disables sharing for that category. Paths must be project-relative
and must not overlap. The helper validates the resulting runtime configuration.
The remote host needs PHP CLI and tar; the archive must contain the deployment
helper. A deployment lock covers upload, preparation, and the atomic current
symlink switch. Rollout progress and helper output stream to stderr; the
deployment name is printed after activation. Uploaded archives are
cached by checksum, so rolling out the same deployment again skips the upload.
Releases use the deployment name (or the archive filename without .tar.gz).
An already-active deployment with the same checksum is a no-op. An inactive
prepared release is reactivated without rerunning the deployment helper.
Reusing a name with different archive contents is rejected.
Old and failed releases are retained; failed preparations require inspection
and explicit cleanup or a new deployment name before retrying.

The deployment helper runs before activation. Migrations may change the shared
database even when preparation fails. This is not a database rollback mechanism.
Configure web serving through current/public; OPcache, long-running workers, and
backward-compatible migrations still need an application-specific strategy for
zero downtime.`,
	Args:              cobra.ExactArgs(1),
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
		return runProjectDeploymentRollout(cmd, target, deployment.Deployment{Reference: args[0]})
	},
}

func runProjectDeploymentRollout(cmd *cobra.Command, backend deployment.Backend, artifact deployment.Deployment) error {
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
