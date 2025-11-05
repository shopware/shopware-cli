package verifier

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

	"github.com/shopware/shopware-cli/extension"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
	"github.com/shopware/shopware-cli/shop"
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
		return nil, fmt.Errorf("shopware/core is not required")
	}

	cst, err := version.NewConstraint(composerJsonData.Require.Shopware)
	if err != nil {
		return nil, err
	}

	return &cst, nil
}

func GetConfigFromProject(root string, onlyLocal bool) (*ToolConfig, error) {
	constraint, err := getShopwareConstraint(root)
	if err != nil {
		return nil, err
	}

	extensions := extension.FindExtensionsFromProject(logging.DisableLogger(context.Background()), root, onlyLocal)

	sourceDirectories := []string{}
	adminDirectories := []string{}
	storefrontDirectories := []string{}

	vendorPath := path.Join(root, "vendor")

	shopCfg, err := shop.ReadConfig(path.Join(root, ".shopware-project.yml"), true)
	if err != nil {
		return nil, err
	}

	excludeExtensions := []string{}

	if shopCfg.Validation != nil {
		for _, ignore := range shopCfg.Validation.IgnoreExtensions {
			excludeExtensions = append(excludeExtensions, ignore.Name)
		}
	}

	for _, ext := range extensions {
		extName, err := ext.GetName()
		if err != nil {
			return nil, err
		}

		rootDir := ext.GetRootDir()

		resolvedPath, err := filepath.EvalSymlinks(rootDir)
		if err == nil {
			rootDir = resolvedPath
		}

		// Skip plugins in vendor folder
		if strings.HasPrefix(rootDir, vendorPath) || slices.Contains(excludeExtensions, extName) {
			continue
		}

		sourceDirectories = append(sourceDirectories, ext.GetSourceDirs()...)
		adminDirectories = append(adminDirectories, getAdminFolders(ext)...)
		storefrontDirectories = append(storefrontDirectories, getStorefrontFolders(ext)...)
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

	for bundlePath := range rootComposerJsonData.Extra.Bundles {
		sourceDirectories = append(sourceDirectories, path.Join(root, bundlePath))

		expectedAdminPath := path.Join(root, bundlePath, "Resources", "app", "administration")
		expectedStorefrontPath := path.Join(root, bundlePath, "Resources", "app", "storefront")

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
		RootDir:               root,
		SourceDirectories:     sourceDirectories,
		AdminDirectories:      adminDirectories,
		StorefrontDirectories: storefrontDirectories,
		ValidationIgnores:     validationIgnores,
	}

	if err := determineVersionRange(toolCfg, constraint); err != nil {
		return nil, err
	}

	return toolCfg, nil
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
