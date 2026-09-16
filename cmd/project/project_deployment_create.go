//go:build deployment

package project

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/projectbuild"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectDeploymentCreateCmd = &cobra.Command{
	Use:   "create [project-directory]",
	Short: "Create an immutable deployment",
	Long: `Create an immutable deployment using the selected environment's backend.
For SSH, build a temporary local copy with the archive packaging pipeline and
retain the tar.gz in .shopware-cli/deployments. No SSH connection is opened.
Print the archive path as the deployment reference. Existing archives are never
overwritten. With no directory, find the closest Shopware project.
This does not upload or roll out the deployment.`,
	Args: cobra.MaximumNArgs(1),
	ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := resolveDeploymentProjectRoot(args)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		withDev, _ := cmd.Flags().GetBool("with-dev-dependencies")
		cmdExecutor, err := resolveProjectDeploymentExecutor(cmd, projectRoot, projectbuild.ArchiveOptions{
			OutputPath: output,
			Build:      projectbuild.Options{WithDevDependencies: withDev, ToolVersion: tui.AppVersion},
		})
		if err != nil {
			return err
		}

		return runProjectDeploymentCreate(cmd, cmdExecutor)
	},
}

func runProjectDeploymentCreate(cmd *cobra.Command, cmdExecutor executor.Executor) error {
	deploymentExecutor, ok := cmdExecutor.(executor.DeploymentExecutor)
	if !ok {
		return fmt.Errorf("creating deployments with executor %q: %w", cmdExecutor.Type(), executor.ErrNotSupported)
	}

	deployment, err := deploymentExecutor.CreateDeployment(cmd.Context())
	if err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}
	if deployment.Reference == "" {
		return errors.New("create deployment: backend returned an empty deployment reference")
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), deployment.Reference)
	return err
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentCreateCmd)
	projectDeploymentCreateCmd.Flags().StringP("output", "o", "", "Local archive path (default: .shopware-cli/deployments/shopware-<unique-id>.tar.gz)")
	projectDeploymentCreateCmd.Flags().Bool("with-dev-dependencies", false, "Install dev dependencies")
}
