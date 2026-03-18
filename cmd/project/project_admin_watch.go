package project

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/npm"
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

		if err := os.Setenv("PROJECT_ROOT", projectRoot); err != nil {
			return err
		}

		adminRelPath := extension.PlatformRelPath(projectRoot, "Administration", "Resources/app/administration")
		adminExecutor := cmdExecutor.WithRelDir(adminRelPath)

		if _, err := os.Stat(extension.PlatformPath(projectRoot, "Administration", "Resources/app/administration/node_modules/webpack-dev-server")); os.IsNotExist(err) {
			if err := npm.InstallDependencies(cmd.Context(), adminExecutor, npm.NonEmptyPackage); err != nil {
				return err
			}
		}

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

			relMockDir, err := filepath.Rel(projectRoot, mockDirectory)

			if err != nil {
				return err
			}

			if err := runTransparentCommand(cmdExecutor.ConsoleCommand(cmd.Context(), "framework:schema", "-s", "entity-schema", filepath.Join(relMockDir, "entity-schema.json"))); err != nil {
				return err
			}

			if err := runTransparentCommand(adminExecutor.NPMCommand(cmd.Context(), "run", "convert-entity-schema")); err != nil {
				return err
			}
		}

		return runTransparentCommand(adminExecutor.NPMCommand(cmd.Context(), "run", "dev"))
	},
}

func init() {
	projectRootCmd.AddCommand(projectAdminWatchCmd)
	projectAdminWatchCmd.PersistentFlags().String("only-extensions", "", "Only watch the given extensions (comma separated)")
	projectAdminWatchCmd.PersistentFlags().String("skip-extensions", "", "Skips the given extensions (comma separated)")
	projectAdminWatchCmd.PersistentFlags().Bool("only-custom-static-extensions", false, "Only build extensions from custom/static-plugins directory")
}
