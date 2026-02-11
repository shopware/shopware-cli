package project

import (
	"os"
	"os/exec"
	"path"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/npm"
	"github.com/shopware/shopware-cli/internal/phpexec"
	"github.com/shopware/shopware-cli/internal/shop"
)

var projectAdminWatchCmd = &cobra.Command{
	Use:     "admin-watch [path]",
	Short:   "Starts the Shopware Admin Watcher",
	Aliases: []string{"watch-admin"},
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

		if err := filterAndWritePluginJson(cmd, projectRoot, shopCfg); err != nil {
			return err
		}

		if err := runTransparentCommand(commandWithRoot(phpexec.ConsoleCommand(cmd.Context(), "feature:dump"), projectRoot)); err != nil {
			return err
		}

		if err := os.Setenv("PROJECT_ROOT", projectRoot); err != nil {
			return err
		}

		if _, err := os.Stat(extension.PlatformPath(projectRoot, "Administration", "Resources/app/administration/node_modules/webpack-dev-server")); os.IsNotExist(err) {
			if err := npm.InstallDependencies(cmd.Context(), extension.PlatformPath(projectRoot, "Administration", "Resources/app/administration"), npm.NonEmptyPackage); err != nil {
				return err
			}
		}

		adminRoot := extension.PlatformPath(projectRoot, "Administration", "Resources/app/administration")

		if err := os.Setenv("ADMIN_ROOT", extension.PlatformPath(projectRoot, "Administration", "")); err != nil {
			return err
		}

		if _, err := os.Stat(extension.PlatformPath(projectRoot, "Administration", "Resources/app/administration/scripts/entitySchemaConverter/entity-schema-converter.ts")); err == nil {
			mockDirectory := extension.PlatformPath(projectRoot, "Administration", "Resources/app/administration/test/_mocks_")
			if _, err := os.Stat(mockDirectory); os.IsNotExist(err) {
				if err := os.MkdirAll(mockDirectory, os.ModePerm); err != nil {
					return err
				}
			}

			if err := runTransparentCommand(commandWithRoot(phpexec.ConsoleCommand(cmd.Context(), "framework:schema", "-s", "entity-schema", path.Join(mockDirectory, "entity-schema.json")), projectRoot)); err != nil {
				return err
			}

			if err := runTransparentCommand(commandWithRoot(exec.CommandContext(cmd.Context(), "npm", "run", "convert-entity-schema"), adminRoot)); err != nil {
				return err
			}
		}

		return runTransparentCommand(commandWithRoot(exec.CommandContext(cmd.Context(), "npm", "run", "dev"), adminRoot))
	},
}

func init() {
	projectRootCmd.AddCommand(projectAdminWatchCmd)
	projectAdminWatchCmd.PersistentFlags().String("only-extensions", "", "Only watch the given extensions (comma separated)")
	projectAdminWatchCmd.PersistentFlags().String("skip-extensions", "", "Skips the given extensions (comma separated)")
	projectAdminWatchCmd.PersistentFlags().Bool("only-custom-static-extensions", false, "Only build extensions from custom/static-plugins directory")
}
