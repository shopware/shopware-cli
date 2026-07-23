package extension

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
)

var extensionAssetBundleCmd = &cobra.Command{
	Use:   "build [path]",
	Short: "Builds assets for extensions",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		assetCfg := extension.AssetBuildConfig{
			ShopwareRoot: os.Getenv("SHOPWARE_PROJECT_ROOT"),
		}
		if assetCfg.ShopwareRoot != "" {
			assetCfg.Executor = executor.NewLocal(assetCfg.ShopwareRoot)
		}
		validatedExtensions := make([]extension.Extension, 0)

		for _, arg := range args {
			path, err := filepath.Abs(arg)
			if err != nil {
				return fmt.Errorf("cannot open file: %w", err)
			}

			ext, err := extension.GetExtensionByFolder(cmd.Context(), path)
			if err != nil {
				return fmt.Errorf("cannot open extension: %w", err)
			}

			validatedExtensions = append(validatedExtensions, ext)
		}

		if assetCfg.ShopwareRoot != "" {
			constraint, err := extension.GetShopwareProjectConstraint(assetCfg.ShopwareRoot)
			if err != nil {
				return fmt.Errorf("cannot get shopware version constraint from project %s: %w", assetCfg.ShopwareRoot, err)
			}
			assetCfg.ShopwareVersion = constraint
		} else {
			constraint, err := extension.GetShopwareVersionConstraintForBuild(validatedExtensions[0])
			if err != nil {
				return fmt.Errorf("cannot get shopware version constraint: %w", err)
			}

			assetCfg.ShopwareVersion = constraint
		}

		if err := extension.BuildAssetsForExtensions(cmd.Context(), extension.ConvertExtensionsToSources(cmd.Context(), validatedExtensions), assetCfg); err != nil {
			return fmt.Errorf("cannot build assets: %w", err)
		}

		return nil
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionAssetBundleCmd)
}
