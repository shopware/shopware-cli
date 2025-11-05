package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/asset"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/phpexec"
	"github.com/shopware/shopware-cli/logging"
	"github.com/shopware/shopware-cli/shop"
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

		return nil, fmt.Errorf("missing shopware/core requirement in composer.json")
	}

	c, err := version.NewConstraint(constraint)
	if err != nil {
		if strings.Contains(err.Error(), "malformed constraint") {
			if _, statErr := os.Stat(path.Join(project, "composer.lock")); os.IsNotExist(statErr) {
				return nil, err
			}

			lock, err := packagist.ReadComposerLock(path.Join(project, "composer.lock"))
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
		return nil, fmt.Errorf("could not determine shopware version")
	}

	matches := kernelFallbackRegExp.FindSubmatch(kernel)

	if len(matches) < 2 {
		return nil, fmt.Errorf("could not determine shopware version")
	}

	v, err := version.NewConstraint(fmt.Sprintf("~%s.0", string(matches[1])))
	if err != nil {
		return nil, err
	}

	return &v, nil
}

// FindAssetSourcesOfProject This finds all assets without invoking any PHP function and thinks all plugins / apps are active. Optional for CI usage.
func FindAssetSourcesOfProject(ctx context.Context, project string, shopCfg *shop.Config) []asset.Source {
	extensions := FindExtensionsFromProject(ctx, project, false)
	sources := ConvertExtensionsToSources(ctx, extensions)

	composerJson, err := os.ReadFile(path.Join(project, "composer.json"))
	if err != nil {
		logging.FromContext(ctx).Errorf("Cannot read composer.json: %s", err.Error())
	}

	var composer rootComposerJson

	err = json.Unmarshal(composerJson, &composer)
	if err != nil {
		logging.FromContext(ctx).Errorf("Cannot parse composer.json: %s", err.Error())
		return sources
	}

	for bundlePath, bundle := range composer.Extra.Bundles {
		name := bundle.Name

		if name == "" {
			name = filepath.Base(bundlePath)
		}

		logging.FromContext(ctx).Infof("Found bundle in project: %s (path: %s)", name, bundlePath)

		bundleConfig, err := readExtensionConfig(bundlePath)
		if err != nil {
			logging.FromContext(ctx).Errorf("Cannot read bundle config: %s", err.Error())
			continue
		}

		sources = append(sources, asset.Source{
			Name:                        name,
			Path:                        path.Join(project, bundlePath),
			AdminEsbuildCompatible:      bundleConfig.Build.Zip.Assets.EnableESBuildForAdmin,
			StorefrontEsbuildCompatible: bundleConfig.Build.Zip.Assets.EnableESBuildForStorefront,
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

func DumpAndLoadAssetSourcesOfProject(ctx context.Context, project string, shopCfg *shop.Config) ([]asset.Source, error) {
	dumpExec := phpexec.ConsoleCommand(ctx, "bundle:dump")
	dumpExec.Dir = project
	dumpExec.Stdin = os.Stdin
	dumpExec.Stdout = os.Stdout
	dumpExec.Stderr = os.Stderr

	if err := dumpExec.Run(); err != nil {
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

			if extensionCfg, err := readExtensionConfig(path.Join(project, pluginsJson[name].BasePath)); err == nil {
				source.AdminEsbuildCompatible = extensionCfg.Build.Zip.Assets.EnableESBuildForAdmin
				source.StorefrontEsbuildCompatible = extensionCfg.Build.Zip.Assets.EnableESBuildForStorefront
				source.NpmStrict = extensionCfg.Build.Zip.Assets.NpmStrict
			}

			sources = append(sources, source)
		}
	}

	return sources, nil
}

func FindExtensionsFromProject(ctx context.Context, project string, onlyLocal bool) []Extension {
	extensions := make(map[string]Extension)

	if !onlyLocal {
		for _, ext := range addExtensionsByComposer(project) {
			name, err := ext.GetName()
			if err != nil {
				continue
			}

			version, _ := ext.GetVersion()

			logging.FromContext(ctx).Infof("Found extension using Composer: %s (%s)", name, version)

			extensions[name] = ext
		}
	}

	for _, ext := range addExtensionsByWildcard(path.Join(project, "custom", "static-plugins")) {
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

	for _, ext := range addExtensionsByWildcard(path.Join(project, "custom", "plugins")) {
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

	for _, ext := range addExtensionsByWildcard(path.Join(project, "custom", "apps")) {
		name, err := ext.GetName()
		if err != nil {
			continue
		}
		version, _ := ext.GetVersion()

		logging.FromContext(ctx).Infof("Found extension in custom/apps: %s (%s)", name, version)

		extensions[name] = ext
	}

	extensionsSlice := make([]Extension, 0, len(extensions))

	for _, ext := range extensions {
		extensionsSlice = append(extensionsSlice, ext)
	}

	return extensionsSlice
}

func addExtensionsByComposer(project string) []Extension {
	var list []Extension

	lock, err := os.ReadFile(path.Join(project, "composer.lock"))
	if err != nil {
		return list
	}

	var composer composerLock
	if err := json.Unmarshal(lock, &composer); err != nil {
		return list
	}

	for _, pkg := range composer.Packages {
		if pkg.PackageType == ComposerTypePlugin || pkg.PackageType == ComposerTypeBundle || pkg.PackageType == ComposerTypeApp {
			ext, err := GetExtensionByFolder(path.Join(project, "vendor", pkg.Name))
			if err != nil {
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

	return list
}

func addExtensionsByWildcard(extensionDir string) []Extension {
	var list []Extension

	extensions, err := os.ReadDir(extensionDir)
	if err != nil {
		return list
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
			ext, err := GetExtensionByFolder(evaluatedPath)
			if err != nil {
				continue
			}

			list = append(list, ext)
		}
	}

	return list
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
