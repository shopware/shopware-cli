//go:build deployment

package project

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
)

var projectDeploymentLogsCmd = &cobra.Command{
	Use:   "logs <deployment-reference>",
	Short: "Show stored deployment-helper logs",
	Long: `Print the stored deployment-helper output for a deployment in the selected environment.
For SSH, use the deployment name or archive path. The local archive is not
required. Logs include stdout and stderr from the original helper run, including
failed runs. Reactivation and already-active deployments do not rerun the helper
or replace these logs. Older deployments created before log recording have no
stored logs.

Output is written directly to stdout and can be redirected to a file.
Logs may contain sensitive information printed by the deployment helper.`,
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
		return runProjectDeploymentLogs(cmd, target, deployment.Deployment{Reference: args[0]})
	},
}

func runProjectDeploymentLogs(cmd *cobra.Command, target deployment.Backend, artifact deployment.Deployment) error {
	backend, ok := target.(deployment.DeploymentLogs)
	if !ok {
		return fmt.Errorf("reading deployment logs with backend %q: %w", target.Type(), deployment.ErrNotSupported)
	}
	if err := backend.WriteDeploymentLogs(cmd.Context(), artifact, cmd.OutOrStdout()); err != nil {
		return fmt.Errorf("read deployment logs: %w", err)
	}
	return nil
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentLogsCmd)
}
