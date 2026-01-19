package project

import (
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/npm"
	"github.com/shopware/shopware-cli/internal/phpexec"
	"github.com/shopware/shopware-cli/internal/shop"
)

var projectStorefrontWatchCmd = &cobra.Command{
	Use:     "storefront-watch [path]",
	Short:   "Starts the Shopware Storefront Watcher",
	Aliases: []string{"watch-storefront"},
	RunE: func(cmd *cobra.Command, args []string) error {
		var projectRoot string
		var err error

		if len(args) == 1 {
			projectRoot = args[0]
		} else if projectRoot, err = findClosestShopwareProject(); err != nil {
			return err
		}

		if err := extension.LoadSymfonyEnvFile(projectRoot); err != nil {
			return err
		}

		shopCfg, err := shop.ReadConfig(projectConfigPath, true)
		if err != nil {
			return err
		}

		if err := filterAndWritePluginJson(cmd, projectRoot, shopCfg); err != nil {
			return err
		}

		if err := runTransparentCommand(commandWithRoot(phpexec.ConsoleCommand(cmd.Context(), "feature:dump"), projectRoot)); err != nil {
			return err
		}

		activeOnly := "--active-only"

		if !themeCompileSupportsActiveOnly(projectRoot) {
			activeOnly = "-v"
		}

		if err := runTransparentCommand(commandWithRoot(phpexec.ConsoleCommand(cmd.Context(), "theme:compile", activeOnly), projectRoot)); err != nil {
			return err
		}

		if err := runTransparentCommand(commandWithRoot(phpexec.ConsoleCommand(cmd.Context(), "theme:dump"), projectRoot)); err != nil {
			return err
		}

		if err := os.Setenv("PROJECT_ROOT", projectRoot); err != nil {
			return err
		}

		if err := os.Setenv("STOREFRONT_ROOT", extension.PlatformPath(projectRoot, "Storefront", "")); err != nil {
			return err
		}

		if _, err := os.Stat(extension.PlatformPath(projectRoot, "Storefront", "Resources/app/storefront/node_modules/webpack-dev-server")); os.IsNotExist(err) {
			if err := npm.InstallDependencies(cmd.Context(), extension.PlatformPath(projectRoot, "Storefront", "Resources/app/storefront"), npm.NonEmptyPackage); err != nil {
				return err
			}
		}

		return runTransparentCommand(commandWithRoot(exec.CommandContext(cmd.Context(), "npm", "run-script", "hot-proxy"), extension.PlatformPath(projectRoot, "Storefront", "Resources/app/storefront")))
	},
}

func themeCompileSupportsActiveOnly(projectRoot string) bool {
	themeFile := extension.PlatformPath(projectRoot, "Storefront", "Theme/Command/ThemeCompileCommand.php")

	bytes, err := os.ReadFile(themeFile)
	if err != nil {
		return false
	}

	return strings.Contains(string(bytes), "active-only")
}

func init() {
	projectRootCmd.AddCommand(projectStorefrontWatchCmd)
	projectStorefrontWatchCmd.PersistentFlags().String("only-extensions", "", "Only watch the given extensions (comma separated)")
	projectStorefrontWatchCmd.PersistentFlags().String("skip-extensions", "", "Skips the given extensions (comma separated)")
	projectStorefrontWatchCmd.PersistentFlags().Bool("only-custom-static-extensions", false, "Only build extensions from custom/static-plugins directory")
}
