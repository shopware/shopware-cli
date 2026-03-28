package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

	paths, err := InstallNodeModulesOfConfigs(ctx, cfgs, assetConfig)
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

			if err := prepareShopwareForAsset(shopwareRoot, nonCompatibleExtensions, assetConfig); err != nil {
				return err
			}

			administrationRoot := PlatformPath(shopwareRoot, "Administration", "Resources/app/administration")
			adminRelPath := PlatformRelPath(shopwareRoot, "Administration", "Resources/app/administration")

			if assetConfig.NPMForceInstall || !npm.NodeModulesExists(administrationRoot) {
				var additionalNpmParameters []string

				npmPackage, err := npm.ReadPackage(administrationRoot)
				if err != nil {
					return err
				}

				if npmPackage.HasDevDependency("puppeteer") {
					additionalNpmParameters = []string{"--production"}
				}

				if err := npm.InstallDependencies(ctx, assetConfig.ExecutorWithRelDir(adminRelPath), npmPackage, additionalNpmParameters...); err != nil {
					return err
				}
			}

			envMap := map[string]string{
				"PROJECT_ROOT": assetConfig.NormalizePath(shopwareRoot),
				"ADMIN_ROOT":   assetConfig.NormalizePath(PlatformPath(shopwareRoot, "Administration", "")),
			}

			if !projectRequiresBuild(shopwareRoot) && !assetConfig.ForceAdminBuild {
				logging.FromContext(ctx).Debugf("Building only administration assets for plugins")
				envMap["SHOPWARE_ADMIN_BUILD_ONLY_EXTENSIONS"] = "1"
				envMap["SHOPWARE_ADMIN_SKIP_SOURCEMAP_GENERATION"] = "1"
			} else {
				logging.FromContext(ctx).Debugf("Building also the administration itself")
			}

			adminExec := assetConfig.ExecutorWithRelDir(adminRelPath).WithEnv(envMap)
			npmBuild := adminExec.NPMCommand(ctx, "run", "build")
			npmBuild.Stdout = os.Stdout
			npmBuild.Stderr = os.Stderr
			err = npmBuild.Run()

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

			if err := prepareShopwareForAsset(shopwareRoot, nonCompatibleExtensions, assetConfig); err != nil {
				return err
			}

			storefrontRoot := PlatformPath(shopwareRoot, "Storefront", "Resources/app/storefront")
			storefrontRelPath := PlatformRelPath(shopwareRoot, "Storefront", "Resources/app/storefront")
			sfExec := assetConfig.ExecutorWithRelDir(storefrontRelPath)

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

				if err := npm.InstallDependencies(ctx, sfExec, npmPackage, additionalNpmParameters...); err != nil {
					return err
				}

				// As we call npm install caniuse-lite, we need to run the postinstall script manually.
				if npmPackage.HasScript("postinstall") {
					npmRunPostInstall := sfExec.NPMCommand(ctx, "run", "postinstall")
					npmRunPostInstall.Stdout = os.Stdout
					npmRunPostInstall.Stderr = os.Stderr

					if err := npmRunPostInstall.Run(); err != nil {
						return err
					}
				}

				if _, err := os.Stat(path.Join(storefrontRoot, "vendor/bootstrap")); os.IsNotExist(err) {
					npmVendor := sfExec.NPMCommand(ctx, "exec", "--", "node", "copy-to-vendor.js")
					npmVendor.Stdout = os.Stdout
					npmVendor.Stderr = os.Stderr
					if err := npmVendor.Run(); err != nil {
						return err
					}
				}
			}

			sfEnvMap := map[string]string{
				"NODE_ENV":        "production",
				"PROJECT_ROOT":    assetConfig.NormalizePath(shopwareRoot),
				"STOREFRONT_ROOT": assetConfig.NormalizePath(storefrontRoot),
			}

			if assetConfig.Browserslist != "" {
				sfEnvMap["BROWSERSLIST"] = assetConfig.Browserslist
			}

			webpackExec := sfExec.WithEnv(sfEnvMap)
			nodeWebpackCmd := webpackExec.NPMCommand(ctx, "exec", "--", "webpack", "--config", "webpack.config.js")
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

func prepareShopwareForAsset(shopwareRoot string, cfgs ExtensionAssetConfig, assetConfig AssetBuildConfig) error {
	varFolder := fmt.Sprintf("%s/var", shopwareRoot)
	if _, err := os.Stat(varFolder); os.IsNotExist(err) {
		err := os.Mkdir(varFolder, os.ModePerm)
		if err != nil {
			return fmt.Errorf("prepareShopwareForAsset: %w", err)
		}
	}

	normalized := make(map[string]*ExtensionAssetConfigEntry, len(cfgs))
	for name, cfg := range cfgs {
		entry := new(ExtensionAssetConfigEntry)
		*entry = ExtensionAssetConfigEntry{
			BasePath:       assetConfig.NormalizePath(cfg.BasePath),
			TechnicalName:  cfg.TechnicalName,
			Administration: cfg.Administration,
			Storefront:     cfg.Storefront,
		}
		entry.Views = make([]string, len(cfg.Views))
		for i, v := range cfg.Views {
			entry.Views[i] = assetConfig.NormalizePath(v)
		}
		normalized[name] = entry
	}

	pluginJson, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("prepareShopwareForAsset: %w", err)
	}

	if err = os.WriteFile(fmt.Sprintf("%s/var/plugins.json", shopwareRoot), pluginJson, os.ModePerm); err != nil {
		return fmt.Errorf("prepareShopwareForAsset: %w", err)
	}

	err = os.WriteFile(fmt.Sprintf("%s/var/features.json", shopwareRoot), []byte("{}"), os.ModePerm)
	if err != nil {
		return fmt.Errorf("prepareShopwareForAsset: %w", err)
	}

	return nil
}
