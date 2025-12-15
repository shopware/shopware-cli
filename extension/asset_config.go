package extension

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/cespare/xxhash/v2"
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
	ForceExtensionBuild          []string
	ForceAdminBuild              bool
	KeepNodeModules              []string
	UseNpmCi                     bool
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

	sumOfFiles string
}

func (e *ExtensionAssetConfigEntry) RequiresBuild() bool {
	return e.Administration.EntryFilePath != nil || e.Storefront.EntryFilePath != nil
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

// GetContentHash returns a cached xxhash of all relevant files in the extension
func (e *ExtensionAssetConfigEntry) GetContentHash() (string, error) {
	if e.sumOfFiles != "" {
		return e.sumOfFiles, nil
	}

	files, err := e.collectFilesForHashing()
	if err != nil {
		return "", fmt.Errorf("failed to collect files: %w", err)
	}

	// Sort files to ensure consistent hashing
	sort.Strings(files)

	// Parallelize file hashing
	type fileHash struct {
		path string
		hash uint64
		err  error
	}

	// Use worker pool pattern for parallel hashing
	numWorkers := 8
	if len(files) < numWorkers {
		numWorkers = len(files)
	}

	fileChan := make(chan string, len(files))
	resultChan := make(chan fileHash, len(files))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range fileChan {
				hash, err := e.hashSingleFile(filePath)
				resultChan <- fileHash{path: filePath, hash: hash, err: err}
			}
		}()
	}

	// Send files to workers
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	// Wait for workers to finish
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results and combine hashes
	fileHashes := make(map[string]uint64)
	for result := range resultChan {
		if result.err != nil {
			return "", fmt.Errorf("failed to hash file %s: %w", result.path, result.err)
		}
		fileHashes[result.path] = result.hash
	}

	// Combine hashes in sorted order for consistency
	hasher := xxhash.New()
	for _, file := range files {
		// Write file path and its hash
		if _, err := hasher.Write([]byte(file)); err != nil {
			return "", err
		}
		if _, err := fmt.Fprintf(hasher, "%x", fileHashes[file]); err != nil {
			return "", err
		}
	}

	e.sumOfFiles = fmt.Sprintf("%x", hasher.Sum64())
	return e.sumOfFiles, nil
}

// collectFilesForHashing collects all relevant files that should be included in the hash
func (e *ExtensionAssetConfigEntry) collectFilesForHashing() ([]string, error) {
	var files []string

	// Collect administration files
	if e.Administration.EntryFilePath != nil {
		adminPath := path.Join(e.BasePath, e.Administration.Path)
		if err := e.collectFilesFromDir(adminPath, &files); err != nil {
			return nil, fmt.Errorf("failed to collect admin files: %w", err)
		}

		// Add webpack config if exists
		if e.Administration.Webpack != nil {
			files = append(files, path.Join(e.BasePath, *e.Administration.Webpack))
		}
	}

	// Collect storefront files
	if e.Storefront.EntryFilePath != nil {
		storefrontPath := path.Join(e.BasePath, e.Storefront.Path)
		if err := e.collectFilesFromDir(storefrontPath, &files); err != nil {
			return nil, fmt.Errorf("failed to collect storefront files: %w", err)
		}

		// Add webpack config if exists
		if e.Storefront.Webpack != nil {
			files = append(files, path.Join(e.BasePath, *e.Storefront.Webpack))
		}

		// Add style files
		for _, styleFile := range e.Storefront.StyleFiles {
			files = append(files, path.Join(e.BasePath, styleFile))
		}
	}

	// Add package.json files
	files = append(files, e.getPossibleNodePaths()...)

	return files, nil
}

// collectFilesFromDir recursively collects all JS, TS, CSS, SCSS files from a directory
func (e *ExtensionAssetConfigEntry) collectFilesFromDir(dir string, files *[]string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and node_modules
		if info.IsDir() || strings.Contains(path, "node_modules") {
			return nil
		}

		// Only include relevant file types
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".js" || ext == ".ts" || ext == ".jsx" || ext == ".tsx" ||
			ext == ".css" || ext == ".scss" || ext == ".sass" || ext == ".less" ||
			ext == ".vue" || ext == ".json" {
			*files = append(*files, path)
		}

		return nil
	})

	return err
}

// hashSingleFile hashes a single file and returns its hash
func (e *ExtensionAssetConfigEntry) hashSingleFile(filePath string) (uint64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		// File might not exist, return zero hash
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer func() {
		_ = file.Close()
	}()

	hasher := xxhash.New()

	// Write the file path to the hasher for uniqueness
	if _, err := hasher.Write([]byte(filePath)); err != nil {
		return 0, err
	}

	// Copy file content to hasher
	if _, err := io.Copy(hasher, file); err != nil {
		return 0, err
	}

	return hasher.Sum64(), nil
}

func (e *ExtensionAssetConfigEntry) GetOutputAdminPath() string {
	return path.Join(e.BasePath, "Resources", "public", "administration")
}

func (e *ExtensionAssetConfigEntry) GetOutputStorefrontPath() string {
	return path.Join(e.BasePath, "Resources", "app", "storefront", "dist", "storefront")
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
