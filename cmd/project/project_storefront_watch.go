package project

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/npm"
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

		shopCfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
		if err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		if err := filterAndWritePluginJson(cmd, projectRoot, shopCfg, cmdExecutor); err != nil {
			return err
		}

		if err := runTransparentCommand(cmdExecutor.ConsoleCommand(cmd.Context(), "feature:dump")); err != nil {
			return err
		}

		activeOnly := "--active-only"

		if !themeCompileSupportsActiveOnly(projectRoot) {
			activeOnly = "-v"
		}

		if err := runTransparentCommand(cmdExecutor.ConsoleCommand(cmd.Context(), "theme:compile", activeOnly)); err != nil {
			return err
		}

		if err := runTransparentCommand(cmdExecutor.ConsoleCommand(cmd.Context(), "theme:dump")); err != nil {
			return err
		}

		if err := os.Setenv("PROJECT_ROOT", projectRoot); err != nil {
			return err
		}

		if err := os.Setenv("STOREFRONT_ROOT", extension.PlatformPath(projectRoot, "Storefront", "")); err != nil {
			return err
		}

		storefrontRelPath := extension.PlatformRelPath(projectRoot, "Storefront", "Resources/app/storefront")
		storefrontExecutor := cmdExecutor.WithRelDir(storefrontRelPath)

		if _, err := os.Stat(extension.PlatformPath(projectRoot, "Storefront", "Resources/app/storefront/node_modules/webpack-dev-server")); os.IsNotExist(err) {
			if err := npm.InstallDependencies(cmd.Context(), storefrontExecutor, npm.NonEmptyPackage); err != nil {
				return err
			}
		}

		return runTransparentCommand(storefrontExecutor.NPMCommand(cmd.Context(), "run-script", "hot-proxy"))
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
