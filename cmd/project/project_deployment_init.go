//go:build deployment

package project

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/system"
)

var projectDeploymentInitCmd = &cobra.Command{
	Use:   "init [project-directory]",
	Short: "Initialize a deployment environment",
	Long: `Interactively bootstrap the selected deployment environment.

For SSH, this creates the remote shared runtime configuration used by releases.
Database credentials and the application URL are written to shared/.env.local.
Optional fresh-install values are stored separately and removed after a
successful Deployment Helper run.

The selected environment must already exist in the project configuration.
With no directory, the closest Shopware project is used.`,
	Args: cobra.MaximumNArgs(1),
	ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if !system.IsInteractionEnabled(cmd.Context()) {
			return errors.New("project deploy init requires interaction")
		}
		root, err := resolveDeploymentProjectRoot(args)
		if err != nil {
			return err
		}
		target, err := resolveProjectDeploymentBackend(cmd, root)
		if err != nil {
			return err
		}
		return runProjectDeploymentInit(cmd, target)
	},
}

func runProjectDeploymentInit(cmd *cobra.Command, target deployment.Backend) error {
	initializer, ok := target.(deployment.Initializer)
	if !ok {
		return fmt.Errorf("initializing deployments with backend %q: %w", target.Type(), deployment.ErrNotSupported)
	}
	if err := initializer.InitializeDeployment(cmd.Context()); err != nil {
		return fmt.Errorf("initialize deployment: %w", err)
	}
	return nil
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentInitCmd)
}
