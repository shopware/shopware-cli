package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/asset"
	"github.com/shopware/shopware-cli/internal/ci"
	"github.com/shopware/shopware-cli/internal/esbuild"
	"github.com/shopware/shopware-cli/logging"
)

const (
	StorefrontWebpackConfig        = "Resources/app/storefront/build/webpack.config.js"
	StorefrontWebpackCJSConfig     = "Resources/app/storefront/build/webpack.config.cjs"
	StorefrontEntrypointJS         = "Resources/app/storefront/src/main.js"
	StorefrontEntrypointTS         = "Resources/app/storefront/src/main.ts"
	StorefrontBaseCSS              = "Resources/app/storefront/src/scss/base.scss"
	AdministrationWebpackConfig    = "Resources/app/administration/build/webpack.config.js"
	AdministrationWebpackCJSConfig = "Resources/app/administration/build/webpack.config.cjs"
	AdministrationEntrypointJS     = "Resources/app/administration/src/main.js"
	AdministrationEntrypointTS     = "Resources/app/administration/src/main.ts"
)

type AssetBuildConfig struct {
	CleanupNodeModules           bool
	DisableAdminBuild            bool
	DisableStorefrontBuild       bool
	ShopwareRoot                 string
	ShopwareVersion              *version.Constraints
	Browserslist                 string
	SkipExtensionsWithBuildFiles bool
	NPMForceInstall              bool
	ContributeProject            bool
	ForceExtensionBuild          []string
	KeepNodeModules              []string
}

func BuildAssetsForExtensions(ctx context.Context, sources []asset.Source, assetConfig AssetBuildConfig) error { // nolint:gocyclo
	cfgs := BuildAssetConfigFromExtensions(ctx, sources, assetConfig)

	if len(cfgs) == 0 {
		return nil
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
			if err := prepareShopwareForAsset(shopwareRoot, nonCompatibleExtensions); err != nil {
				return err
			}

			administrationRoot := PlatformPath(shopwareRoot, "Administration", "Resources/app/administration")

			if assetConfig.NPMForceInstall || !nodeModulesExists(administrationRoot) {
				var additionalNpmParameters []string

				npmPackage, err := getNpmPackage(administrationRoot)
				if err != nil {
					return err
				}

				if doesPackageJsonContainsPackageInDev(npmPackage, "puppeteer") {
					additionalNpmParameters = []string{"--production"}
				}

				if err := InstallNPMDependencies(ctx, administrationRoot, npmPackage, additionalNpmParameters...); err != nil {
					return err
				}
			}

			envList := []string{fmt.Sprintf("PROJECT_ROOT=%s", shopwareRoot), fmt.Sprintf("ADMIN_ROOT=%s", PlatformPath(shopwareRoot, "Administration", ""))}

			if !assetConfig.ContributeProject {
				envList = append(envList, "SHOPWARE_ADMIN_BUILD_ONLY_EXTENSIONS=1", "SHOPWARE_ADMIN_SKIP_SOURCEMAP_GENERATION=1")
			}

			err = npmRunBuild(
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
			nonCompatibleExtensions["Storefront"] = ExtensionAssetConfigEntry{
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

			if assetConfig.NPMForceInstall || !nodeModulesExists(storefrontRoot) {
				if err := patchPackageLockToRemoveCanIUsePackage(path.Join(storefrontRoot, "package-lock.json")); err != nil {
					return err
				}

				additionalNpmParameters := []string{"caniuse-lite"}

				npmPackage, err := getNpmPackage(storefrontRoot)
				if err != nil {
					return err
				}

				if doesPackageJsonContainsPackageInDev(npmPackage, "puppeteer") {
					additionalNpmParameters = append(additionalNpmParameters, "--production")
				}

				if err := InstallNPMDependencies(ctx, storefrontRoot, npmPackage, additionalNpmParameters...); err != nil {
					return err
				}

				// As we call npm install caniuse-lite, we need to run the postinstal script manually.
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

	return nil
}

func nodeModulesExists(root string) bool {
	if _, err := os.Stat(path.Join(root, "node_modules")); err == nil {
		return true
	}

	return false
}

type npmInstallJob struct {
	npmPath             string
	additionalNpmParams []string
	additionalText      string
}

type npmInstallResult struct {
	nodeModulesPath string
	err             error
}

func InstallNodeModulesOfConfigs(ctx context.Context, cfgs ExtensionAssetConfig, force bool) ([]string, error) {
	// Collect all npm install jobs
	jobs := make([]npmInstallJob, 0)

	addedJobs := make(map[string]bool)

	// Install shared node_modules between admin and storefront
	for _, entry := range cfgs {
		possibleNodePaths := []string{
			// shared between admin and storefront
			path.Join(entry.BasePath, "Resources", "app", "package.json"),
			path.Join(entry.BasePath, "package.json"),
			path.Join(path.Dir(entry.BasePath), "package.json"),
			path.Join(path.Dir(path.Dir(entry.BasePath)), "package.json"),
			path.Join(path.Dir(path.Dir(path.Dir(entry.BasePath))), "package.json"),
		}

		// only try administration and storefront node_modules folder when we have an entry file
		if entry.Administration.EntryFilePath != nil {
			possibleNodePaths = append(possibleNodePaths, path.Join(entry.BasePath, "Resources", "app", "administration", "package.json"), path.Join(entry.BasePath, "Resources", "app", "administration", "src", "package.json"))
		}

		if entry.Storefront.EntryFilePath != nil {
			possibleNodePaths = append(possibleNodePaths, path.Join(entry.BasePath, "Resources", "app", "storefront", "package.json"), path.Join(entry.BasePath, "Resources", "app", "storefront", "src", "package.json"))
		}

		additionalNpmParameters := []string{}

		if entry.NpmStrict {
			additionalNpmParameters = []string{"--production"}
		}

		for _, possibleNodePath := range possibleNodePaths {
			if _, err := os.Stat(possibleNodePath); err == nil {
				npmPath := path.Dir(possibleNodePath)

				if !force && nodeModulesExists(npmPath) {
					continue
				}

				additionalText := ""
				if !entry.NpmStrict {
					additionalText = " (consider enabling npm_strict mode, to install only production relevant dependencies)"
				}

				if !addedJobs[npmPath] {
					addedJobs[npmPath] = true
				} else {
					continue
				}

				jobs = append(jobs, npmInstallJob{
					npmPath:             npmPath,
					additionalNpmParams: additionalNpmParameters,
					additionalText:      additionalText,
				})
			}
		}
	}

	if len(jobs) == 0 {
		return []string{}, nil
	}

	// Set up worker pool with number of CPU cores
	numWorkers := runtime.NumCPU()
	jobChan := make(chan npmInstallJob, len(jobs))
	resultChan := make(chan npmInstallResult, len(jobs))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				result := processNpmInstallJob(ctx, job)
				resultChan <- result
			}
		}()
	}

	// Send jobs to workers
	for _, job := range jobs {
		jobChan <- job
	}
	close(jobChan)

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	paths := make([]string, 0)
	for result := range resultChan {
		if result.err != nil {
			return nil, result.err
		}
		if result.nodeModulesPath != "" {
			paths = append(paths, result.nodeModulesPath)
		}
	}

	return paths, nil
}

func processNpmInstallJob(ctx context.Context, job npmInstallJob) npmInstallResult {
	npmPackage, err := getNpmPackage(job.npmPath)
	if err != nil {
		return npmInstallResult{err: err}
	}

	logging.FromContext(ctx).Infof("Installing npm dependencies in %s %s\n", job.npmPath, job.additionalText)

	if err := InstallNPMDependencies(ctx, job.npmPath, npmPackage, job.additionalNpmParams...); err != nil {
		return npmInstallResult{err: err}
	}

	return npmInstallResult{
		nodeModulesPath: path.Join(job.npmPath, "node_modules"),
	}
}

func deletePaths(ctx context.Context, nodeModulesPaths ...string) {
	for _, nodeModulesPath := range nodeModulesPaths {
		if err := os.RemoveAll(nodeModulesPath); err != nil {
			logging.FromContext(ctx).Errorf("Failed to remove path %s: %s", nodeModulesPath, err.Error())
			return
		}
	}
}

func npmRunBuild(ctx context.Context, path string, buildCmd string, buildEnvVariables []string) error {
	npmBuildCmd := exec.CommandContext(ctx, "npm", "--prefix", path, "run", buildCmd) //nolint:gosec
	npmBuildCmd.Env = os.Environ()
	npmBuildCmd.Env = append(npmBuildCmd.Env, buildEnvVariables...)
	npmBuildCmd.Stdout = os.Stdout
	npmBuildCmd.Stderr = os.Stderr

	if err := npmBuildCmd.Run(); err != nil {
		return err
	}

	return nil
}

func InstallNPMDependencies(ctx context.Context, path string, packageJsonData NpmPackage, additionalParams ...string) error {
	isProductionMode := false

	for _, param := range additionalParams {
		if param == "--production" {
			isProductionMode = true
		}
	}

	if isProductionMode && len(packageJsonData.Dependencies) == 0 {
		return nil
	}

	installCmd := exec.CommandContext(ctx, "npm", "install", "--no-audit", "--no-fund", "--prefer-offline", "--loglevel=error")
	installCmd.Args = append(installCmd.Args, additionalParams...)
	installCmd.Dir = path
	installCmd.Env = os.Environ()
	installCmd.Env = append(installCmd.Env, "PUPPETEER_SKIP_DOWNLOAD=1", "NPM_CONFIG_ENGINE_STRICT=false", "NPM_CONFIG_FUND=false", "NPM_CONFIG_AUDIT=false", "NPM_CONFIG_UPDATE_NOTIFIER=false")

	combinedOutput, err := installCmd.CombinedOutput()
	if err != nil {
		logging.FromContext(context.Background()).Errorf("npm install failed in %s: %s", path, string(combinedOutput))
		return fmt.Errorf("installing dependencies for %s failed with error: %w", path, err)
	}

	return nil
}

func getNpmPackage(root string) (NpmPackage, error) {
	packageJsonFile, err := os.ReadFile(path.Join(root, "package.json"))
	if err != nil {
		return NpmPackage{}, err
	}

	var packageJsonData NpmPackage
	if err := json.Unmarshal(packageJsonFile, &packageJsonData); err != nil {
		return NpmPackage{}, err
	}
	return packageJsonData, nil
}

func prepareShopwareForAsset(shopwareRoot string, cfgs map[string]ExtensionAssetConfigEntry) error {
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

func BuildAssetConfigFromExtensions(ctx context.Context, sources []asset.Source, assetCfg AssetBuildConfig) ExtensionAssetConfig {
	list := make(ExtensionAssetConfig)

	for _, source := range sources {
		if source.Name == "" {
			continue
		}

		resourcesDir := path.Join(source.Path, "Resources", "app")

		if _, err := os.Stat(resourcesDir); os.IsNotExist(err) {
			continue
		}

		absPath, err := filepath.EvalSymlinks(source.Path)
		if err != nil {
			logging.FromContext(ctx).Errorf("Could not resolve symlinks for %s: %s", source.Path, err.Error())
			continue
		}

		absPath, err = filepath.Abs(absPath)
		if err != nil {
			logging.FromContext(ctx).Errorf("Could not get absolute path for %s: %s", source.Path, err.Error())
			continue
		}

		sourceConfig := createConfigFromPath(source.Name, absPath)
		sourceConfig.EnableESBuildForAdmin = source.AdminEsbuildCompatible
		sourceConfig.EnableESBuildForStorefront = source.StorefrontEsbuildCompatible
		sourceConfig.DisableSass = source.DisableSass
		sourceConfig.NpmStrict = source.NpmStrict

		if assetCfg.SkipExtensionsWithBuildFiles {
			expectedAdminCompiledFile := path.Join(source.Path, "Resources", "public", "administration", "js", esbuild.ToKebabCase(source.Name)+".js")
			expectedAdminVitePath := path.Join(source.Path, "Resources", "public", "administration", ".vite", "manifest.json")
			expectedStorefrontCompiledFile := path.Join(source.Path, "Resources", "app", "storefront", "dist", "storefront", "js", esbuild.ToKebabCase(source.Name), esbuild.ToKebabCase(source.Name)+".js")

			// Check if extension is in the ForceExtensionBuild list
			forceExtensionBuild := slices.Contains(assetCfg.ForceExtensionBuild, source.Name)

			_, foundAdminCompiled := os.Stat(expectedAdminCompiledFile)
			_, foundAdminVite := os.Stat(expectedAdminVitePath)
			_, foundStorefrontCompiled := os.Stat(expectedStorefrontCompiledFile)

			if (foundAdminCompiled == nil || foundAdminVite == nil) && !forceExtensionBuild {
				// clear out the entrypoint, so the admin does not build it
				sourceConfig.Administration.EntryFilePath = nil
				sourceConfig.Administration.Webpack = nil

				logging.FromContext(ctx).Infof("Skipping building administration assets for \"%s\" as compiled files are present", source.Name)
			}

			if foundStorefrontCompiled == nil && !forceExtensionBuild {
				// clear out the entrypoint, so the storefront does not build it
				sourceConfig.Storefront.EntryFilePath = nil
				sourceConfig.Storefront.Webpack = nil

				logging.FromContext(ctx).Infof("Skipping building storefront assets for \"%s\" as compiled files are present", source.Name)
			}
		}

		list[source.Name] = sourceConfig
	}

	return list
}

func createConfigFromPath(entryPointName string, extensionRoot string) ExtensionAssetConfigEntry {
	var entryFilePathAdmin, entryFilePathStorefront, webpackFileAdmin, webpackFileStorefront *string
	storefrontStyles := make([]string, 0)

	if _, err := os.Stat(path.Join(extensionRoot, AdministrationEntrypointJS)); err == nil {
		val := AdministrationEntrypointJS
		entryFilePathAdmin = &val
	}

	if _, err := os.Stat(path.Join(extensionRoot, AdministrationEntrypointTS)); err == nil {
		val := AdministrationEntrypointTS
		entryFilePathAdmin = &val
	}

	if _, err := os.Stat(path.Join(extensionRoot, AdministrationWebpackConfig)); err == nil {
		val := AdministrationWebpackConfig
		webpackFileAdmin = &val
	}

	if _, err := os.Stat(path.Join(extensionRoot, AdministrationWebpackCJSConfig)); err == nil {
		val := AdministrationWebpackCJSConfig
		webpackFileAdmin = &val
	}

	if _, err := os.Stat(path.Join(extensionRoot, StorefrontEntrypointJS)); err == nil {
		val := StorefrontEntrypointJS
		entryFilePathStorefront = &val
	}

	if _, err := os.Stat(path.Join(extensionRoot, StorefrontEntrypointTS)); err == nil {
		val := StorefrontEntrypointTS
		entryFilePathStorefront = &val
	}

	if _, err := os.Stat(path.Join(extensionRoot, StorefrontWebpackConfig)); err == nil {
		val := StorefrontWebpackConfig
		webpackFileStorefront = &val
	}

	if _, err := os.Stat(path.Join(extensionRoot, StorefrontWebpackCJSConfig)); err == nil {
		val := StorefrontWebpackCJSConfig
		webpackFileStorefront = &val
	}

	if _, err := os.Stat(path.Join(extensionRoot, StorefrontBaseCSS)); err == nil {
		storefrontStyles = append(storefrontStyles, StorefrontBaseCSS)
	}

	extensionRoot = strings.TrimRight(extensionRoot, "/") + "/"

	cfg := ExtensionAssetConfigEntry{
		BasePath: extensionRoot,
		Views: []string{
			"Resources/views",
		},
		TechnicalName: esbuild.ToKebabCase(entryPointName),
		Administration: ExtensionAssetConfigAdmin{
			Path:          "Resources/app/administration/src",
			EntryFilePath: entryFilePathAdmin,
			Webpack:       webpackFileAdmin,
		},
		Storefront: ExtensionAssetConfigStorefront{
			Path:          "Resources/app/storefront/src",
			EntryFilePath: entryFilePathStorefront,
			Webpack:       webpackFileStorefront,
			StyleFiles:    storefrontStyles,
		},
	}
	return cfg
}

func setupShopwareInTemp(ctx context.Context, minVersion string) (string, error) {
	dir, err := os.MkdirTemp("", "extension")
	if err != nil {
		return "", err
	}

	branch := "v" + strings.ToLower(minVersion)

	if minVersion == DevVersionNumber || minVersion == "6.7.0.0" {
		branch = "trunk"
	}

	logging.FromContext(ctx).Infof("Cloning shopware with branch: %s into %s", branch, dir)

	gitCheckoutCmd := exec.CommandContext(ctx, "git", "clone", "https://github.com/shopware/shopware.git", "--depth=1", "-b", branch, dir)
	gitCheckoutCmd.Stdout = os.Stdout
	gitCheckoutCmd.Stderr = os.Stderr
	err = gitCheckoutCmd.Run()
	if err != nil {
		return "", err
	}

	return dir, nil
}

type ExtensionAssetConfig map[string]ExtensionAssetConfigEntry

func (c ExtensionAssetConfig) Has(name string) bool {
	_, ok := c[name]

	return ok
}

func (c ExtensionAssetConfig) RequiresShopwareRepository() bool {
	for _, entry := range c {
		if entry.Administration.EntryFilePath != nil && !entry.EnableESBuildForAdmin {
			return true
		}

		if entry.Storefront.EntryFilePath != nil && !entry.EnableESBuildForStorefront {
			return true
		}
	}

	return false
}

func (c ExtensionAssetConfig) RequiresAdminBuild() bool {
	for _, entry := range c {
		if entry.Administration.EntryFilePath != nil {
			return true
		}
	}

	return false
}

func (c ExtensionAssetConfig) RequiresStorefrontBuild() bool {
	for _, entry := range c {
		if entry.Storefront.EntryFilePath != nil {
			return true
		}
	}

	return false
}

func (c ExtensionAssetConfig) FilterByAdmin() ExtensionAssetConfig {
	filtered := make(ExtensionAssetConfig)

	for name, entry := range c {
		if entry.Administration.EntryFilePath != nil {
			filtered[name] = entry
		}
	}

	return filtered
}

func (c ExtensionAssetConfig) FilterByAdminAndEsBuild(esbuildEnabled bool) ExtensionAssetConfig {
	filtered := make(ExtensionAssetConfig)

	for name, entry := range c {
		if entry.Administration.EntryFilePath != nil && entry.EnableESBuildForAdmin == esbuildEnabled {
			filtered[name] = entry
		}
	}

	return filtered
}

func (c ExtensionAssetConfig) FilterByStorefrontAndEsBuild(esbuildEnabled bool) ExtensionAssetConfig {
	filtered := make(ExtensionAssetConfig)

	for name, entry := range c {
		if entry.Storefront.EntryFilePath != nil && entry.EnableESBuildForStorefront == esbuildEnabled {
			filtered[name] = entry
		}
	}

	return filtered
}

func (c ExtensionAssetConfig) Only(extensions []string) ExtensionAssetConfig {
	filtered := make(ExtensionAssetConfig)

	for name, entry := range c {
		if slices.Contains(extensions, name) {
			filtered[name] = entry
		}
	}

	return filtered
}

func (c ExtensionAssetConfig) Not(extensions []string) ExtensionAssetConfig {
	filtered := make(ExtensionAssetConfig)

	for name, entry := range c {
		if !slices.Contains(extensions, name) {
			filtered[name] = entry
		}
	}

	return filtered
}

type ExtensionAssetConfigEntry struct {
	BasePath                   string                         `json:"basePath"`
	Views                      []string                       `json:"views"`
	TechnicalName              string                         `json:"technicalName"`
	Administration             ExtensionAssetConfigAdmin      `json:"administration"`
	Storefront                 ExtensionAssetConfigStorefront `json:"storefront"`
	EnableESBuildForAdmin      bool
	EnableESBuildForStorefront bool
	DisableSass                bool
	NpmStrict                  bool
}

type ExtensionAssetConfigAdmin struct {
	Path          string  `json:"path"`
	EntryFilePath *string `json:"entryFilePath"`
	Webpack       *string `json:"webpack"`
}

type ExtensionAssetConfigStorefront struct {
	Path          string   `json:"path"`
	EntryFilePath *string  `json:"entryFilePath"`
	Webpack       *string  `json:"webpack"`
	StyleFiles    []string `json:"styleFiles"`
}

func doesPackageJsonContainsPackageInDev(packageJsonData NpmPackage, packageName string) bool {
	if _, ok := packageJsonData.DevDependencies[packageName]; ok {
		return true
	}

	return false
}
