//go:build deployment

package project

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/projectbuild"
	"github.com/shopware/shopware-cli/internal/shop"
)

var projectDeploymentPackageContainerCmd = &cobra.Command{
	Use:   "container [project-directory]",
	Short: "Generate container build files and print the Docker build command",
	Long: `Write a standalone Dockerfile and .dockerignore into the project root for
committing and building manually, then print a docker buildx build command.
The Dockerfile builds with the Shopware CLI image and uses the Shopware
FrankenPHP runtime image. Existing files are never overwritten.

Without a directory, find the closest Shopware project. PHP defaults to the
project's php_version, then docker.php.version, then the highest supported
version matching Shopware's PHP requirement in composer.lock, otherwise 8.3.
--php-version overrides detection. Docker is not required to generate the files.
No credentials or resolved configuration are written.

The CLI never builds, loads, pushes, or runs an image. --load, --push, --tag,
and --platform only customize the printed command. With neither --load nor
--push, the selected builder's default output behavior applies when you run it.
Generated file paths go to stderr; stdout contains the command to run.`,
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
		if _, err := cfg.ResolveEnvironment(environmentName); err != nil {
			return err
		}
		phpVersion, _ := cmd.Flags().GetString("php-version")
		tags, _ := cmd.Flags().GetStringArray("tag")
		platform, _ := cmd.Flags().GetString("platform")
		withDev, _ := cmd.Flags().GetBool("with-dev-dependencies")
		load, _ := cmd.Flags().GetBool("load")
		push, _ := cmd.Flags().GetBool("push")
		opts := projectbuild.ContainerOptions{
			PHPVersion:          phpVersion,
			Tags:                tags,
			Platform:            platform,
			Load:                load,
			Push:                push,
			WithDevDependencies: withDev,
		}
		paths, err := projectbuild.GenerateContainerFiles(cmd.Context(), root, cfg, opts)
		if err != nil {
			return err
		}
		for _, path := range paths {
			if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "Generated %s\n", path); err != nil {
				return err
			}
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), projectbuild.ContainerBuildCommand(root, opts))
		return err
	},
}

func init() {
	projectDeploymentPackageCmd.AddCommand(projectDeploymentPackageContainerCmd)
	projectDeploymentPackageContainerCmd.Flags().String("php-version", "", "PHP image version (default: project PHP/Docker config, composer.lock, then 8.3)")
	projectDeploymentPackageContainerCmd.Flags().StringArrayP("tag", "t", nil, "Image name and optional tag (repeatable)")
	projectDeploymentPackageContainerCmd.Flags().String("platform", "", "Target Docker platform, e.g. linux/amd64")
	projectDeploymentPackageContainerCmd.Flags().Bool("load", false, "Include --load in the printed Docker command")
	projectDeploymentPackageContainerCmd.Flags().Bool("push", false, "Include --push in the printed Docker command")
	projectDeploymentPackageContainerCmd.Flags().Bool("with-dev-dependencies", false, "Install dev dependencies")
	_ = projectDeploymentPackageContainerCmd.RegisterFlagCompletionFunc("php-version", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return shop.SupportedPHPVersions, cobra.ShellCompDirectiveNoFileComp
	})
}
