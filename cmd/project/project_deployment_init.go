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
	Args:  cobra.MaximumNArgs(1),
	ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	},
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		if !system.IsInteractionEnabled(cmd.Context()) {
			return errors.New("cannot initialize deployment: project deploy init requires interaction")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolveDeploymentProjectRoot(args)
		if err != nil {
			return err
		}
		target, err := deployment.ResolveBackend(cmd.Context(), root, projectConfigPath, environmentName)
		if err != nil {
			return err
		}
		return runProjectDeploymentInit(cmd, target)
	},
}

// runProjectDeploymentInit initializes a supported backend and prints its setup guidance after success.
func runProjectDeploymentInit(cmd *cobra.Command, target deployment.Backend) error {
	initializer, ok := target.(deployment.Initializer)
	if !ok {
		return fmt.Errorf("cannot initialize deployments with backend %q: %w", target.Type(), deployment.ErrNotSupported)
	}
	if err := initializer.InitializeDeployment(cmd.Context()); err != nil {
		if errors.Is(err, deployment.ErrInitializationCanceled) {
			return nil
		}
		return fmt.Errorf("cannot initialize deployment: %w", err)
	}
	if guide, ok := target.(deployment.InitializationGuide); ok {
		environment, _ := cmd.Flags().GetString("env")
		return guide.WriteInitializationNextSteps(environment, cmd.OutOrStdout())
	}
	return nil
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentInitCmd)
}
