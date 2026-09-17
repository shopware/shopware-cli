//go:build deployment

package project

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/projectbuild"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectDeploymentPackageArchiveCmd = &cobra.Command{
	Use:   "archive [project-directory]",
	Short: "Build a deployment archive in a temporary copy of the project",
	Long: `Run the project CI build pipeline in a temporary copy and package the result
as tar.gz. Without a directory, find the closest Shopware project.

The source checkout is not built in place. Git metadata, local environment files,
runtime data, and Composer credentials are not included in the archive. The
packaged project config contains only deployment settings. Build hooks still run
as trusted project code; this is not a sandbox.

By default, create a unique archive in the project's .shopware-cli/deployments
directory. Use --output for an explicit path relative to the current directory.
Existing artifacts are never overwritten. The archive path is printed after
the build finishes. Nothing is uploaded or rolled out.`,
	Args: cobra.MaximumNArgs(1),
	ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolveDeploymentProjectRoot(args)
		if err != nil {
			return err
		}
		configPath := packageProjectConfigPath(cmd, root)
		cfg, err := shop.ReadConfig(cmd.Context(), configPath, true)
		if err != nil {
			return err
		}
		env, err := cfg.ResolveEnvironment(environmentName)
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
		reference, err := projectbuild.PackageArchive(cmd.Context(), root, cfg, env, projectbuild.ArchiveOptions{
			OutputPath: output,
			ConfigPath: configPath,
			Build: projectbuild.Options{
				WithDevDependencies: withDev,
				ToolVersion:         tui.AppVersion,
			},
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), reference)
		return err
	},
}

func init() {
	projectDeploymentPackageCmd.AddCommand(projectDeploymentPackageArchiveCmd)
	projectDeploymentPackageArchiveCmd.Flags().StringP("output", "o", "", "Archive path (default: .shopware-cli/deployments/shopware-<unique-id>.tar.gz in the project)")
	projectDeploymentPackageArchiveCmd.Flags().Bool("with-dev-dependencies", false, "Install dev dependencies")
}
