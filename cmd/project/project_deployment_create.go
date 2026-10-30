//go:build deployment

package project

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/cliversion"
	"github.com/shopware/shopware-cli/internal/deployment"
	deploymentbackend "github.com/shopware/shopware-cli/internal/deployment/backend"
)

var projectDeploymentCreateCmd = &cobra.Command{
	Use:   "create [project-directory]",
	Short: "Create an immutable deployment archive",
	Long: `Create an immutable deployment using the selected environment's backend.
Requires an environment with SSH backend. Build a temporary local copy using the archive
packaging pipeline and retain the tar.gz under a generated name in
.shopware-cli/deployments. Creation does not connect to SSH or upload anything.

Report the generated name as the deployment reference. With --output, report the
absolute archive path instead; relative output paths use the working directory.
Existing archives are never overwritten. Without a directory, find the closest
Shopware project.`,
	Example: `  shopware-cli project deploy create -e production
  shopware-cli project deploy create ./shop -e production --output ./builds/shop.tar.gz`,
	Args: cobra.MaximumNArgs(1),
	ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolveDeploymentProjectRoot(args)
		if err != nil {
			return err
		}
		backend, err := deploymentbackend.ResolveBackend(cmd.Context(), root, projectConfigPath, environmentName)
		if err != nil {
			return err
		}
		output, err := cmd.Flags().GetString("output")
		if err != nil {
			return err
		}
		withDev, err := cmd.Flags().GetBool("with-dev-dependencies")
		if err != nil {
			return err
		}
		artifact, err := backend.CreateDeployment(cmd.Context(), deployment.CreateOptions{
			OutputPath: output, WithDevDependencies: withDev, ToolVersion: cliversion.Version,
		})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Created deployment %q\n", artifact.Reference); err != nil {
			return fmt.Errorf("cannot report created deployment: %w", err)
		}
		return nil
	},
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentCreateCmd)
	projectDeploymentCreateCmd.Flags().StringP("output", "o", "", "Local archive path (default: .shopware-cli/deployments/<generated-name>.tar.gz)")
	projectDeploymentCreateCmd.Flags().Bool("with-dev-dependencies", false, "Install dev dependencies")
}
