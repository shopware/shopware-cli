package extension

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	gocomposer "github.com/shyim/go-composer"
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/asset"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

func GetShopwareProjectConstraint(project string) (*version.Constraints, error) {
	composerJson, err := os.ReadFile(path.Join(project, "composer.json"))
	if err != nil {
		return nil, fmt.Errorf("could not read composer.json: %w", err)
	}

	var composer rootComposerJson

	err = json.Unmarshal(composerJson, &composer)
	if err != nil {
		return nil, fmt.Errorf("could not parse composer.json: %w", err)
	}

	constraint, ok := composer.Require["shopware/core"]

	if !ok {
		if v, err := getProjectConstraintFromKernel(project); err == nil {
			return v, nil
		}

		return nil, errors.New("missing shopware/core requirement in composer.json")
	}

	c, err := version.NewConstraint(constraint)
	if err != nil {
		if strings.Contains(err.Error(), "malformed constraint") {
			if _, statErr := os.Stat(path.Join(project, "composer.lock")); os.IsNotExist(statErr) {
				return nil, err
			}

			lock, err := gocomposer.ReadLock(path.Join(project, "composer.lock"))
			if err != nil {
				return nil, err
			}

			for _, pkg := range lock.Packages {
				if pkg.Name == "shopware/core" {
					v, err := version.NewConstraint(pkg.Version)
					if err != nil {
						return getProjectConstraintFromKernel(project)
					}

					return &v, nil
				}
			}
		}

		return nil, err
	}

	return &c, nil
}

var kernelFallbackRegExp = regexp.MustCompile(`(?m)SHOPWARE_FALLBACK_VERSION\s*=\s*'?"?(\d+\.\d+)`)

func getProjectConstraintFromKernel(project string) (*version.Constraints, error) {
	kernelPath := PlatformPath(project, "Core", "Kernel.php")

	kernel, err := os.ReadFile(kernelPath)
	if err != nil {
		return nil, errors.New("could not determine shopware version")
	}

	matches := kernelFallbackRegExp.FindSubmatch(kernel)

	if len(matches) < 2 {
		return nil, errors.New("could not determine shopware version")
	}

	v, err := version.NewConstraint(fmt.Sprintf("~%s.0", string(matches[1])))
	if err != nil {
		return nil, err
	}

	return &v, nil
}

// FindAssetSourcesOfProject This finds all assets without invoking any PHP function and thinks all plugins / apps are active. Optional for CI usage.
func FindAssetSourcesOfProject(ctx context.Context, project string, shopCfg *shop.Config) []asset.Source {
	extensions, _ := FindExtensionsFromProject(ctx, project, false)
	sources := ConvertExtensionsToSources(ctx, extensions)

	composerJson, err := os.ReadFile(path.Join(project, "composer.json"))
	if err != nil {
		logging.FromContext(ctx).Errorf("Cannot read composer.json: %s", err.Error())
		return sources
	}

	var composer rootComposerJson

	err = json.Unmarshal(composerJson, &composer)
	if err != nil {
		logging.FromContext(ctx).Errorf("Cannot parse composer.json: %s", err.Error())
		return sources
	}

	// Deprecated: Loading bundles from composer.json extra.shopware-bundles is deprecated.
	// Use the build.bundles section in .config/shopware-project.yml instead.
	seenPaths := make(map[string]bool)
	for bundlePath, bundle := range composer.Extra.Bundles {
		name := bundle.Name

		if name == "" {
			name = filepath.Base(bundlePath)
		}

		logging.FromContext(ctx).Warnf("Deprecation: Bundle %q is configured via composer.json extra.shopware-bundles. Please move it to the build.bundles section in .config/shopware-project.yml instead.", bundlePath)
		logging.FromContext(ctx).Infof("Found bundle in project: %s (path: %s)", name, bundlePath)

		bundleConfig, err := readExtensionConfig(ctx, bundlePath)
		if err != nil {
			logging.FromContext(ctx).Errorf("Cannot read bundle config: %s", err.Error())
			continue
		}

		sources = append(sources, asset.Source{
			Name:                        name,
			Path:                        path.Join(project, bundlePath),
			AdminEsbuildCompatible:      bundleConfig.Build.Zip.Assets.EnableESBuildForAdmin,
			StorefrontEsbuildCompatible: bundleConfig.Build.Zip.Assets.EnableESBuildForStorefront,
			AdditionalCaches:            convertAdditionalCaches(bundleConfig.Build.Zip.Assets.AdditionalCaches),
		})
		seenPaths[bundlePath] = true
	}

	for _, bundle := range shopCfg.Build.Bundles {
		if seenPaths[bundle.Path] {
			continue
		}
		name := bundle.Name
		if name == "" {
			name = filepath.Base(bundle.Path)
		}
		logging.FromContext(ctx).Infof("Found bundle in project config: %s (path: %s)", name, bundle.Path)
		bundleConfig, err := readExtensionConfig(ctx, bundle.Path)
		if err != nil {
			logging.FromContext(ctx).Errorf("Cannot read bundle config: %s", err.Error())
			continue
		}
		sources = append(sources, asset.Source{
			Name:                        name,
			Path:                        path.Join(project, bundle.Path),
			AdminEsbuildCompatible:      bundleConfig.Build.Zip.Assets.EnableESBuildForAdmin,
			StorefrontEsbuildCompatible: bundleConfig.Build.Zip.Assets.EnableESBuildForStorefront,
			AdditionalCaches:            convertAdditionalCaches(bundleConfig.Build.Zip.Assets.AdditionalCaches),
		})
	}

	if len(shopCfg.Build.ExcludeExtensions) > 0 {
		logging.FromContext(ctx).Infof("Excluded extensions in project: %s", shopCfg.Build.ExcludeExtensions)
		for _, excludedExtension := range shopCfg.Build.ExcludeExtensions {
			for i, source := range sources {
				if source.Name == excludedExtension {
					sources = append(sources[:i], sources[i+1:]...)
				}
			}
		}
	}

	return sources
}

type ConsoleCommandFunc func(ctx context.Context, args ...string) *exec.Cmd

func DumpAndLoadAssetSourcesOfProject(ctx context.Context, project string, shopCfg *shop.Config, consoleCommand ConsoleCommandFunc) ([]asset.Source, error) {
	dumpExec := consoleCommand(ctx, "bundle:dump")
	dumpExec.Dir = project
	// Capture output: bundle:dump's "Dumped plugin configuration." line corrupts the dev TUI render if inherited.
	var dumpOutput bytes.Buffer
	dumpExec.Stdout = &dumpOutput
	dumpExec.Stderr = &dumpOutput

	if err := dumpExec.Run(); err != nil {
		if out := strings.TrimSpace(dumpOutput.String()); out != "" {
			return nil, fmt.Errorf("could not bundle features: %w: %s", err, out)
		}
		return nil, fmt.Errorf("could not bundle features: %w", err)
	}

	var pluginsJson map[string]ExtensionAssetConfigEntry

	pluginJsonBytes, err := os.ReadFile(path.Join(project, "var", "plugins.json"))
	if err != nil {
		return nil, fmt.Errorf("could not read plugins.json: %w", err)
	}

	if err := json.Unmarshal(pluginJsonBytes, &pluginsJson); err != nil {
		return nil, fmt.Errorf("could not parse plugins.json: %w", err)
	}

	var sources []asset.Source

	for name := range pluginsJson {
		if pluginsJson[name].Administration.EntryFilePath != nil || pluginsJson[name].Storefront.EntryFilePath != nil {
			source := asset.Source{
				Name: name,
				Path: pluginsJson[name].BasePath,
			}

			if extensionCfg, err := readExtensionConfig(ctx, path.Join(project, pluginsJson[name].BasePath)); err == nil {
				source.AdminEsbuildCompatible = extensionCfg.Build.Zip.Assets.EnableESBuildForAdmin
				source.StorefrontEsbuildCompatible = extensionCfg.Build.Zip.Assets.EnableESBuildForStorefront
				source.NpmStrict = extensionCfg.Build.Zip.Assets.NpmStrict
				source.AdditionalCaches = convertAdditionalCaches(extensionCfg.Build.Zip.Assets.AdditionalCaches)
			}

			sources = append(sources, source)
		}
	}

	return sources, nil
}

// FindExtensionsFromProject returns successfully loaded extensions in name order
// along with any load errors. Callers that can use partial results may ignore the error.
// Composer package failures and invalid extension configs are reported; arbitrary
// folders under custom/ that are not extensions are ignored.
func FindExtensionsFromProject(ctx context.Context, project string, onlyLocal bool) ([]Extension, error) {
	extensions := make(map[string]Extension)
	var loadErrors []error

	if !onlyLocal {
		composerExtensions, err := addExtensionsByComposer(ctx, project)
		loadErrors = append(loadErrors, err)
		for _, ext := range composerExtensions {
			name, err := ext.GetName()
			if err != nil {
				continue
			}

			version, _ := ext.GetVersion()

			logging.FromContext(ctx).Infof("Found extension using Composer: %s (%s)", name, version)

			extensions[name] = ext
		}
	}

	staticPlugins, err := addExtensionsByWildcard(ctx, path.Join(project, "custom", "static-plugins"))
	loadErrors = append(loadErrors, err)
	for _, ext := range staticPlugins {
		name, err := ext.GetName()
		if err != nil {
			continue
		}

		// Skip if extension is already added by composer
		if _, ok := extensions[name]; ok {
			continue
		}

		version, _ := ext.GetVersion()

		logging.FromContext(ctx).Infof("Found extension in custom/static-plugins: %s (%s)", name, version)

		extensions[name] = ext
	}

	plugins, err := addExtensionsByWildcard(ctx, path.Join(project, "custom", "plugins"))
	loadErrors = append(loadErrors, err)
	for _, ext := range plugins {
		name, err := ext.GetName()
		if err != nil {
			continue
		}

		// Skip if extension is already added by composer
		if _, ok := extensions[name]; ok {
			continue
		}

		version, _ := ext.GetVersion()

		logging.FromContext(ctx).Infof("Found extension in custom/plugins: %s (%s)", name, version)

		extensions[name] = ext
	}

	apps, err := addExtensionsByWildcard(ctx, path.Join(project, "custom", "apps"))
	loadErrors = append(loadErrors, err)
	for _, ext := range apps {
		name, err := ext.GetName()
		if err != nil {
			continue
		}
		version, _ := ext.GetVersion()

		logging.FromContext(ctx).Infof("Found extension in custom/apps: %s (%s)", name, version)

		extensions[name] = ext
	}

	extensionsSlice := make([]Extension, 0, len(extensions))

	for _, name := range slices.Sorted(maps.Keys(extensions)) {
		extensionsSlice = append(extensionsSlice, extensions[name])
	}

	return extensionsSlice, errors.Join(loadErrors...)
}

// ComposerPackageError reports an extension listed in composer.lock whose
// installed vendor folder cannot be loaded.
type ComposerPackageError struct {
	Package string
	Err     error
}

func (e *ComposerPackageError) Error() string {
	return "package " + e.Package + ": " + e.Err.Error()
}

func (e *ComposerPackageError) Unwrap() error {
	return e.Err
}

func addExtensionsByComposer(ctx context.Context, project string) ([]Extension, error) {
	var list []Extension
	var loadErrors []error

	lock, err := os.ReadFile(path.Join(project, "composer.lock"))
	if errors.Is(err, os.ErrNotExist) {
		return list, nil
	}
	if err != nil {
		return list, fmt.Errorf("cannot read composer.lock: %w", err)
	}

	var composer composerLock
	if err := json.Unmarshal(lock, &composer); err != nil {
		return list, fmt.Errorf("cannot parse composer.lock: %w", err)
	}

	for _, pkg := range composer.Packages {
		if pkg.PackageType == ComposerTypePlugin || pkg.PackageType == ComposerTypeBundle || pkg.PackageType == ComposerTypeApp {
			folder := path.Join(project, "vendor", pkg.Name)
			if _, err := os.Stat(folder); errors.Is(err, os.ErrNotExist) {
				// The package is locked but not installed, so there is nothing to load.
				continue
			}

			ext, err := GetExtensionByFolder(ctx, folder)
			if err != nil {
				loadErrors = append(loadErrors, &ComposerPackageError{Package: pkg.Name, Err: err})
				continue
			}

			// The extension in the vendor folder has maybe not filled the version in this composer.json. Let's overwrite it with the version from composer.lock
			switch pkg.PackageType {
			case ComposerTypePlugin:
				ext.(*PlatformPlugin).Composer.Version = pkg.Version
			case ComposerTypeApp:
				ext.(*App).manifest.Meta.Version = pkg.Version
			case ComposerTypeBundle:
				ext.(*ShopwareBundle).Composer.Version = pkg.Version
			}

			list = append(list, ext)
		}
	}

	return list, errors.Join(loadErrors...)
}

func addExtensionsByWildcard(ctx context.Context, extensionDir string) ([]Extension, error) {
	var list []Extension
	var loadErrors []error

	extensions, err := os.ReadDir(extensionDir)
	if err != nil {
		return list, nil
	}

	for _, file := range extensions {
		extensionPath := path.Join(extensionDir, file.Name())
		evaluatedPath, err := filepath.EvalSymlinks(extensionPath)
		if err != nil {
			continue
		}

		isDir := file.IsDir()

		if evaluatedPath != extensionPath {
			evaluatedStat, err := os.Stat(evaluatedPath)
			if err != nil {
				continue
			}

			isDir = evaluatedStat.IsDir()
		}

		if isDir {
			ext, err := GetExtensionByFolder(ctx, evaluatedPath)
			if err != nil {
				// Wildcard searches also encounter folders that are not extensions.
				// Only a failure to load a recognized extension's config is fatal.
				var configErr *ConfigError
				if errors.As(err, &configErr) {
					loadErrors = append(loadErrors, err)
				}
				continue
			}

			list = append(list, ext)
		}
	}

	return list, errors.Join(loadErrors...)
}

type composerLock struct {
	Packages []struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		PackageType string `json:"type"`
	} `json:"packages"`
}

type rootComposerJson struct {
	Require map[string]string `json:"require"`
	Extra   struct {
		Bundles map[string]rootShopwareBundle `json:"shopware-bundles"`
	}
}

type rootShopwareBundle struct {
	Name string `json:"name"`
}
