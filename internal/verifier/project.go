package verifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

func IsProject(root string) bool {
	composerJson := path.Join(root, "composer.json")

	if _, err := os.Stat(composerJson); os.IsNotExist(err) {
		return false
	}

	var composerJsonData struct {
		Type string `json:"type"`
	}

	file, err := os.Open(composerJson)
	if err != nil {
		return false
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close composer.json: %w", closeErr)
		}
	}()

	if err := json.NewDecoder(file).Decode(&composerJsonData); err != nil {
		return false
	}

	return composerJsonData.Type == "project"
}

func getShopwareConstraint(root string) (*version.Constraints, error) {
	composerJson := path.Join(root, "composer.json")

	file, err := os.Open(composerJson)
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close composer.json: %w", closeErr)
		}
	}()

	var composerJsonData struct {
		Require struct {
			Shopware string `json:"shopware/core"`
		} `json:"require"`
	}

	if err := json.NewDecoder(file).Decode(&composerJsonData); err != nil {
		return nil, err
	}

	if composerJsonData.Require.Shopware == "" {
		return nil, errors.New("shopware/core is not required")
	}

	cst, err := version.NewConstraint(composerJsonData.Require.Shopware)
	if err != nil {
		return nil, err
	}

	return &cst, nil
}

func GetConfigFromProject(ctx context.Context, root string, onlyLocal bool) (*ToolConfig, error) {
	constraint, err := getShopwareConstraint(root)
	if err != nil {
		return nil, err
	}

	extensions := extension.FindExtensionsFromProject(ctx, root, onlyLocal)

	sourceDirectories := []string{}
	adminDirectories := []string{}
	storefrontDirectories := []string{}

	vendorPath := validation.ResolveSourceRoot(path.Join(root, "vendor"))

	actualProjectConfigPath := shop.SearchConfigPath(ctx, root, "")
	shopCfg, err := shop.ReadConfig(ctx, actualProjectConfigPath, true)
	if err != nil {
		return nil, err
	}

	excludeExtensions := []string{}

	if shopCfg.Validation != nil {
		for _, ignore := range shopCfg.Validation.IgnoreExtensions {
			excludeExtensions = append(excludeExtensions, ignore.Name)
		}
	}

	checkExtensions := []extension.Extension{}

	for _, ext := range extensions {
		extName, err := ext.GetName()
		if err != nil {
			return nil, err
		}

		rootDir := validation.ResolveSourceRoot(ext.GetRootDir())

		// Skip plugins in vendor folder
		if (rootDir == vendorPath || strings.HasPrefix(rootDir, vendorPath+string(filepath.Separator))) || slices.Contains(excludeExtensions, extName) {
			continue
		}

		sourceDirectories = append(sourceDirectories, ext.GetSourceDirs()...)
		adminDirectories = append(adminDirectories, getAdminFolders(ext)...)
		storefrontDirectories = append(storefrontDirectories, getStorefrontFolders(ext)...)
		checkExtensions = append(checkExtensions, ext)
	}

	var rootComposerJsonData rootComposerJson

	rootComposerJsonPath := path.Join(root, "composer.json")

	file, err := os.Open(rootComposerJsonPath)
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close composer.json: %w", closeErr)
		}
	}()

	if err := json.NewDecoder(file).Decode(&rootComposerJsonData); err != nil {
		return nil, err
	}

	// Deprecated: Loading bundles from composer.json extra.shopware-bundles is deprecated.
	// Use the build.bundles section in .config/shopware-project.yml instead.
	seenBundlePaths := make(map[string]bool)
	for bundlePath := range rootComposerJsonData.Extra.Bundles {
		logging.FromContext(ctx).Warnf("Deprecation: Bundle %q is configured via composer.json extra.shopware-bundles. Please move it to the build.bundles section in %s instead.", bundlePath, shopCfg.GetStorageLocation())
		sourceDirectories = append(sourceDirectories, path.Join(root, bundlePath))

		expectedAdminPath := path.Join(root, bundlePath, "Resources", "app", "administration")
		expectedStorefrontPath := path.Join(root, bundlePath, "Resources", "app", "storefront")

		if _, err := os.Stat(expectedAdminPath); err == nil {
			adminDirectories = append(adminDirectories, expectedAdminPath)
		}

		if _, err := os.Stat(expectedStorefrontPath); err == nil {
			storefrontDirectories = append(storefrontDirectories, expectedStorefrontPath)
		}
		seenBundlePaths[bundlePath] = true
	}

	for _, bundle := range shopCfg.Build.Bundles {
		if seenBundlePaths[bundle.Path] {
			continue
		}
		sourceDirectories = append(sourceDirectories, path.Join(root, bundle.Path))

		expectedAdminPath := path.Join(root, bundle.Path, "Resources", "app", "administration")
		expectedStorefrontPath := path.Join(root, bundle.Path, "Resources", "app", "storefront")

		if _, err := os.Stat(expectedAdminPath); err == nil {
			adminDirectories = append(adminDirectories, expectedAdminPath)
		}

		if _, err := os.Stat(expectedStorefrontPath); err == nil {
			storefrontDirectories = append(storefrontDirectories, expectedStorefrontPath)
		}
	}

	var validationIgnores []validation.ToolConfigIgnore

	if shopCfg.Validation != nil {
		for _, ignore := range shopCfg.Validation.Ignore {
			validationIgnores = append(validationIgnores, validation.ToolConfigIgnore{
				Identifier: ignore.Identifier,
				Path:       ignore.Path,
				Message:    ignore.Message,
			})
		}
	}

	toolCfg := &ToolConfig{
		ToolDirectory:         GetToolDirectory(),
		InputWasDirectory:     true,
		RootDir:               root,
		SourceDirectories:     sourceDirectories,
		AdminDirectories:      adminDirectories,
		StorefrontDirectories: storefrontDirectories,
		ValidationIgnores:     validationIgnores,
		Extensions:            checkExtensions,
		PHPVersion:            projectPHPVersion(shopCfg, rootComposerJsonData),
	}

	if err := determineVersionRange(toolCfg, constraint); err != nil {
		return nil, err
	}

	toolCfg.logConfiguration(ctx)
	return toolCfg, nil
}

// projectPHPVersion returns the PHP version the project's extensions are
// linted with: validation.php_version from the project config, then
// php_version, then docker.php.version, then
// config.platform.php from composer.json, then the lowest supported version
// allowed by require.php. It returns an empty string when none is set.
func projectPHPVersion(shopCfg *shop.Config, composerJson rootComposerJson) string {
	if shopCfg.Validation != nil && shopCfg.Validation.PhpVersion != "" {
		return shopCfg.Validation.PhpVersion
	}
	if shopCfg.PHPVersion != "" {
		return shopCfg.PHPVersion
	}
	if shopCfg.Docker != nil && shopCfg.Docker.PHP != nil && shopCfg.Docker.PHP.Version != "" {
		return shopCfg.Docker.PHP.Version
	}
	if composerJson.Config.Platform.PHP != "" {
		return composerJson.Config.Platform.PHP
	}
	if constraint := composerJson.Require["php"]; constraint != "" {
		parsed, err := version.NewConstraint(constraint)
		if err != nil {
			return ""
		}
		for _, candidate := range shop.SupportedPHPVersions {
			v, err := version.NewVersion(candidate + ".0")
			if err == nil && parsed.Check(v) {
				return candidate
			}
		}
	}
	return ""
}

type rootComposerJson struct {
	Require map[string]string `json:"require"`
	Config  struct {
		Platform struct {
			PHP string `json:"php"`
		} `json:"platform"`
	} `json:"config"`
	Extra struct {
		Bundles map[string]rootShopwareBundle `json:"shopware-bundles"`
	}
}

type rootShopwareBundle struct {
	Name string `json:"name"`
}
