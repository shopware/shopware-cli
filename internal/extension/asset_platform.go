package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/asset"
	"github.com/shopware/shopware-cli/internal/ci"
	"github.com/shopware/shopware-cli/internal/esbuild"
	"github.com/shopware/shopware-cli/internal/npm"
	"github.com/shopware/shopware-cli/logging"
)

func BuildAssetsForExtensions(ctx context.Context, sources []asset.Source, assetConfig AssetBuildConfig) error { // nolint:gocyclo
	cfgs := BuildAssetConfigFromExtensions(ctx, sources, assetConfig)

	if len(cfgs) == 0 {
		return nil
	}

	if err := restoreAssetCaches(ctx, cfgs, assetConfig); err != nil {
		return err
	}

	if !cfgs.RequiresAdminBuild() && !cfgs.RequiresStorefrontBuild() {
		logging.FromContext(ctx).Infof("Building assets has been skipped as not required")
		return nil
	}

	minVersion, err := lookupForMinMatchingVersion(ctx, assetConfig.ShopwareVersion)
	if err != nil {
		return err
	}

	requiresShopwareSources := cfgs.RequiresShopwareRepository()

	shopwareRoot := assetConfig.ShopwareRoot
	if shopwareRoot == "" && requiresShopwareSources {
		shopwareRoot, err = setupShopwareInTemp(ctx, minVersion)
		if err != nil {
			return err
		}

		defer deletePaths(ctx, shopwareRoot)
	}

	nodeInstallSection := ci.Default.Section(ctx, "Installing node_modules for extensions")

	paths, err := InstallNodeModulesOfConfigs(ctx, cfgs, assetConfig.NPMForceInstall)
	if err != nil {
		return err
	}

	nodeInstallSection.End(ctx)

	if shopwareRoot != "" && len(assetConfig.KeepNodeModules) > 0 {
		paths = slices.DeleteFunc(paths, func(path string) bool {
			rel, err := filepath.Rel(shopwareRoot, path)
			if err != nil {
				return false
			}

			return slices.Contains(assetConfig.KeepNodeModules, rel)
		})
	}

	defer deletePaths(ctx, paths...)

	if !assetConfig.DisableAdminBuild && cfgs.RequiresAdminBuild() {
		administrationSection := ci.Default.Section(ctx, "Building administration assets")

		// Build all extensions compatible with esbuild first
		for name, entry := range cfgs.FilterByAdminAndEsBuild(true) {
			options := esbuild.NewAssetCompileOptionsAdmin(name, entry.BasePath)
			options.DisableSass = entry.DisableSass

			if _, err := esbuild.CompileExtensionAsset(ctx, options); err != nil {
				return err
			}

			if err := esbuild.DumpViteConfig(options); err != nil {
				return err
			}

			logging.FromContext(ctx).Infof("Building administration assets for %s using ESBuild", name)
		}

		nonCompatibleExtensions := cfgs.FilterByAdminAndEsBuild(false)

		if len(nonCompatibleExtensions) != 0 {
			if projectRequiresBuild(shopwareRoot) {
				// add the storefront itself as plugin into json
				var basePath string
				if shopwareRoot == "" {
					basePath = "src/Storefront/"
				} else {
					basePath = strings.TrimLeft(
						strings.Replace(PlatformPath(shopwareRoot, "Storefront", ""), shopwareRoot, "", 1),
						"/",
					) + "/"
				}

				storefrontEntryPath := "Resources/app/storefront/src/main.js"
				adminEntryPath := "Resources/app/administration/src/main.js"
				nonCompatibleExtensions["Storefront"] = &ExtensionAssetConfigEntry{
					BasePath:      basePath,
					Views:         []string{"Resources/views"},
					TechnicalName: "storefront",
					Storefront: ExtensionAssetConfigStorefront{
						Path:          "Resources/app/storefront/src",
						EntryFilePath: &storefrontEntryPath,
						StyleFiles:    []string{},
					},
					Administration: ExtensionAssetConfigAdmin{
						Path:          "Resources/app/administration/src",
						EntryFilePath: &adminEntryPath,
					},
				}
			}

			if err := prepareShopwareForAsset(shopwareRoot, nonCompatibleExtensions); err != nil {
				return err
			}

			administrationRoot := PlatformPath(shopwareRoot, "Administration", "Resources/app/administration")

			if assetConfig.NPMForceInstall || !npm.NodeModulesExists(administrationRoot) {
				var additionalNpmParameters []string

				npmPackage, err := npm.ReadPackage(administrationRoot)
				if err != nil {
					return err
				}

				if npmPackage.HasDevDependency("puppeteer") {
					additionalNpmParameters = []string{"--production"}
				}

				if err := npm.InstallDependencies(ctx, administrationRoot, npmPackage, additionalNpmParameters...); err != nil {
					return err
				}
			}

			envList := []string{fmt.Sprintf("PROJECT_ROOT=%s", shopwareRoot), fmt.Sprintf("ADMIN_ROOT=%s", PlatformPath(shopwareRoot, "Administration", ""))}

			if !projectRequiresBuild(shopwareRoot) && !assetConfig.ForceAdminBuild {
				logging.FromContext(ctx).Debugf("Building only administration assets for plugins")
				envList = append(envList, "SHOPWARE_ADMIN_BUILD_ONLY_EXTENSIONS=1", "SHOPWARE_ADMIN_SKIP_SOURCEMAP_GENERATION=1")
			} else {
				logging.FromContext(ctx).Debugf("Building also the administration itself")
			}

			err = npm.RunScript(
				ctx,
				administrationRoot,
				"build",
				envList,
			)

			if assetConfig.CleanupNodeModules {
				defer deletePaths(ctx, path.Join(administrationRoot, "node_modules"), path.Join(administrationRoot, "twigVuePlugin"))
			}

			if err != nil {
				return err
			}

			for name, entry := range nonCompatibleExtensions {
				options := esbuild.NewAssetCompileOptionsAdmin(name, entry.BasePath)
				if err := esbuild.DumpViteConfig(options); err != nil {
					return err
				}
			}
		}

		administrationSection.End(ctx)
	}

	if !assetConfig.DisableStorefrontBuild && cfgs.RequiresStorefrontBuild() {
		storefrontSection := ci.Default.Section(ctx, "Building storefront assets")
		// Build all extensions compatible with esbuild first
		for name, entry := range cfgs.FilterByStorefrontAndEsBuild(true) {
			isNewLayout := false

			if minVersion == DevVersionNumber || version.Must(version.NewVersion(minVersion)).GreaterThanOrEqual(version.Must(version.NewVersion("6.6.0.0"))) {
				isNewLayout = true
			}

			options := esbuild.NewAssetCompileOptionsStorefront(name, entry.BasePath, isNewLayout)

			if _, err := esbuild.CompileExtensionAsset(ctx, options); err != nil {
				return err
			}
			logging.FromContext(ctx).Infof("Building storefront assets for %s using ESBuild", name)
		}

		nonCompatibleExtensions := cfgs.FilterByStorefrontAndEsBuild(false)

		if len(nonCompatibleExtensions) != 0 {
			// add the storefront itself as plugin into json
			var basePath string
			if shopwareRoot == "" {
				basePath = "src/Storefront/"
			} else {
				basePath = strings.TrimLeft(
					strings.Replace(PlatformPath(shopwareRoot, "Storefront", ""), shopwareRoot, "", 1),
					"/",
				) + "/"
			}

			entryPath := "Resources/app/storefront/src/main.js"
			nonCompatibleExtensions["Storefront"] = &ExtensionAssetConfigEntry{
				BasePath:      basePath,
				Views:         []string{"Resources/views"},
				TechnicalName: "storefront",
				Storefront: ExtensionAssetConfigStorefront{
					Path:          "Resources/app/storefront/src",
					EntryFilePath: &entryPath,
					StyleFiles:    []string{},
				},
				Administration: ExtensionAssetConfigAdmin{
					Path: "Resources/app/administration/src",
				},
			}

			if err := prepareShopwareForAsset(shopwareRoot, nonCompatibleExtensions); err != nil {
				return err
			}

			storefrontRoot := PlatformPath(shopwareRoot, "Storefront", "Resources/app/storefront")

			if assetConfig.NPMForceInstall || !npm.NodeModulesExists(storefrontRoot) {
				if err := npm.PatchPackageLockToRemoveCanIUse(path.Join(storefrontRoot, "package-lock.json")); err != nil {
					return err
				}

				additionalNpmParameters := []string{"caniuse-lite"}

				npmPackage, err := npm.ReadPackage(storefrontRoot)
				if err != nil {
					return err
				}

				if npmPackage.HasDevDependency("puppeteer") {
					additionalNpmParameters = append(additionalNpmParameters, "--production")
				}

				if err := npm.InstallDependencies(ctx, storefrontRoot, npmPackage, additionalNpmParameters...); err != nil {
					return err
				}

				// As we call npm install caniuse-lite, we need to run the postinstall script manually.
				if npmPackage.HasScript("postinstall") {
					npmRunPostInstall := exec.CommandContext(ctx, "npm", "run", "postinstall")
					npmRunPostInstall.Dir = storefrontRoot
					npmRunPostInstall.Stdout = os.Stdout
					npmRunPostInstall.Stderr = os.Stderr

					if err := npmRunPostInstall.Run(); err != nil {
						return err
					}
				}

				if _, err := os.Stat(path.Join(storefrontRoot, "vendor/bootstrap")); os.IsNotExist(err) {
					npmVendor := exec.CommandContext(ctx, "node", path.Join(storefrontRoot, "copy-to-vendor.js"))
					npmVendor.Dir = storefrontRoot
					npmVendor.Stdout = os.Stdout
					npmVendor.Stderr = os.Stderr
					if err := npmVendor.Run(); err != nil {
						return err
					}
				}
			}

			envList := []string{
				"NODE_ENV=production",
				fmt.Sprintf("PROJECT_ROOT=%s", shopwareRoot),
				fmt.Sprintf("STOREFRONT_ROOT=%s", storefrontRoot),
			}

			if assetConfig.Browserslist != "" {
				envList = append(envList, fmt.Sprintf("BROWSERSLIST=%s", assetConfig.Browserslist))
			}

			nodeWebpackCmd := exec.CommandContext(ctx, "node", "node_modules/.bin/webpack", "--config", "webpack.config.js")
			nodeWebpackCmd.Dir = storefrontRoot
			nodeWebpackCmd.Env = os.Environ()
			nodeWebpackCmd.Env = append(nodeWebpackCmd.Env, envList...)
			nodeWebpackCmd.Stdout = os.Stdout
			nodeWebpackCmd.Stderr = os.Stderr

			if err := nodeWebpackCmd.Run(); err != nil {
				return err
			}

			if assetConfig.CleanupNodeModules {
				defer deletePaths(ctx, path.Join(storefrontRoot, "node_modules"))
			}

			if err != nil {
				return err
			}
		}

		storefrontSection.End(ctx)
	}

	if err := storeAssetCaches(ctx, cfgs, assetConfig); err != nil {
		return err
	}

	return nil
}

func prepareShopwareForAsset(shopwareRoot string, cfgs ExtensionAssetConfig) error {
	varFolder := fmt.Sprintf("%s/var", shopwareRoot)
	if _, err := os.Stat(varFolder); os.IsNotExist(err) {
		err := os.Mkdir(varFolder, os.ModePerm)
		if err != nil {
			return fmt.Errorf("prepareShopwareForAsset: %w", err)
		}
	}

	pluginJson, err := json.Marshal(cfgs)
	if err != nil {
		return fmt.Errorf("prepareShopwareForAsset: %w", err)
	}

	err = os.WriteFile(fmt.Sprintf("%s/var/plugins.json", shopwareRoot), pluginJson, os.ModePerm)
	if err != nil {
		return fmt.Errorf("prepareShopwareForAsset: %w", err)
	}

	err = os.WriteFile(fmt.Sprintf("%s/var/features.json", shopwareRoot), []byte("{}"), os.ModePerm)
	if err != nil {
		return fmt.Errorf("prepareShopwareForAsset: %w", err)
	}

	return nil
}
