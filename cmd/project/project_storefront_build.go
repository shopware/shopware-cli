package project

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/phpexec"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

var projectStorefrontBuildCmd = &cobra.Command{
	Use:     "storefront-build [path]",
	Short:   "Builds the Storefront",
	Aliases: []string{"build-storefront"},
	RunE: func(cmd *cobra.Command, args []string) error {
		var projectRoot string
		var err error

		if len(args) == 1 {
			// We need an absolute path for webpack
			projectRoot, err = filepath.Abs(args[0])
			if err != nil {
				return err
			}
		} else if projectRoot, err = findClosestShopwareProject(); err != nil {
			return err
		}

		shopCfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Looking for extensions to build assets in project")

		if err := runTransparentCommand(commandWithRoot(phpexec.ConsoleCommand(phpexec.AllowBinCI(cmd.Context()), "feature:dump"), projectRoot)); err != nil {
			return err
		}

		sources, err := filterAndGetSources(cmd, projectRoot, shopCfg)
		if err != nil {
			return err
		}

		forceInstall, _ := cmd.PersistentFlags().GetBool("force-install-dependencies")

		shopwareConstraint, err := extension.GetShopwareProjectConstraint(projectRoot)
		if err != nil {
			return err
		}

		assetCfg := extension.AssetBuildConfig{
			DisableAdminBuild: true,
			ShopwareRoot:      projectRoot,
			ShopwareVersion:   shopwareConstraint,
			NPMForceInstall:   forceInstall,
		}

		if err := extension.BuildAssetsForExtensions(cmd.Context(), sources, assetCfg); err != nil {
			return err
		}

		skipThemeCompile, _ := cmd.PersistentFlags().GetBool("skip-theme-compile")
		if skipThemeCompile {
			return nil
		}

		return runTransparentCommand(commandWithRoot(phpexec.ConsoleCommand(phpexec.AllowBinCI(cmd.Context()), "theme:compile"), projectRoot))
	},
}

func init() {
	projectRootCmd.AddCommand(projectStorefrontBuildCmd)
	projectStorefrontBuildCmd.PersistentFlags().Bool("skip-theme-compile", false, "Skip theme compilation")
	projectStorefrontBuildCmd.PersistentFlags().Bool("force-install-dependencies", false, "Force install NPM dependencies")
	projectStorefrontBuildCmd.PersistentFlags().String("only-extensions", "", "Only watch the given extensions (comma separated)")
	projectStorefrontBuildCmd.PersistentFlags().String("skip-extensions", "", "Skips the given extensions (comma separated)")
	projectStorefrontBuildCmd.PersistentFlags().Bool("only-custom-static-extensions", false, "Only build extensions from custom/static-plugins directory")
}
