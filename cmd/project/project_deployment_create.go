//go:build deployment

package project

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectDeploymentCreateCmd = &cobra.Command{
	Use:   "create [project-directory]",
	Short: "Create an immutable deployment",
	Long: `Create an immutable deployment using the selected environment's backend.
For SSH, build a temporary local copy with the archive packaging pipeline and
retain the tar.gz under a memorable name such as focused-wise-turing in
.shopware-cli/deployments. No SSH connection is opened. Report that name as the
deployment reference; a custom --output instead reports its archive path.
Existing archives are never overwritten. With no directory, find the closest Shopware project.
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
		backend, err := resolveProjectDeploymentBackend(cmd, projectRoot)
		if err != nil {
			return err
		}

		return runProjectDeploymentCreate(cmd, backend, deployment.CreateOptions{
			OutputPath: output, WithDevDependencies: withDev, ToolVersion: tui.AppVersion,
		})
	},
}

func runProjectDeploymentCreate(cmd *cobra.Command, backend deployment.Backend, options deployment.CreateOptions) error {
	artifact, err := backend.CreateDeployment(cmd.Context(), options)
	if err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}
	if artifact.Reference == "" {
		return errors.New("create deployment: backend returned an empty deployment reference")
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created deployment %q\n", artifact.Reference)
	return err
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentCreateCmd)
	projectDeploymentCreateCmd.Flags().StringP("output", "o", "", "Local archive path (default: .shopware-cli/deployments/<generated-name>.tar.gz)")
	projectDeploymentCreateCmd.Flags().Bool("with-dev-dependencies", false, "Install dev dependencies")
}
