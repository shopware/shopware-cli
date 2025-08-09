package extension

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/asset"
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

type ExtensionAssetConfig map[string]*ExtensionAssetConfigEntry

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

	// internal cache
	cachedPossibleNodePaths []string
	once                    sync.Once
}

func (e *ExtensionAssetConfigEntry) getPossibleNodePaths() []string {
	e.once.Do(func() {
		possibleNodePaths := []string{
			// shared between admin and storefront
			path.Join(e.BasePath, "Resources", "app", "package.json"),
			path.Join(e.BasePath, "package.json"),
			path.Join(path.Dir(e.BasePath), "package.json"),
			path.Join(path.Dir(path.Dir(e.BasePath)), "package.json"),
			path.Join(path.Dir(path.Dir(path.Dir(e.BasePath))), "package.json"),
		}

		// only try administration and storefront node_modules folder when we have an entry file
		if e.Administration.EntryFilePath != nil {
			possibleNodePaths = append(possibleNodePaths, path.Join(e.BasePath, "Resources", "app", "administration", "package.json"), path.Join(e.BasePath, "Resources", "app", "administration", "src", "package.json"))
		}

		if e.Storefront.EntryFilePath != nil {
			possibleNodePaths = append(possibleNodePaths, path.Join(e.BasePath, "Resources", "app", "storefront", "package.json"), path.Join(e.BasePath, "Resources", "app", "storefront", "src", "package.json"))
		}

		existingPaths := make([]string, 0)

		for _, possibleNodePath := range possibleNodePaths {
			if _, err := os.Stat(possibleNodePath); err == nil {
				existingPaths = append(existingPaths, possibleNodePath)
			}
		}

		e.cachedPossibleNodePaths = existingPaths
	})

	return e.cachedPossibleNodePaths
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

func createConfigFromPath(entryPointName string, extensionRoot string) *ExtensionAssetConfigEntry {
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
	return &cfg
}
