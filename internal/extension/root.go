package extension

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/archiver"
	"github.com/shopware/shopware-cli/internal/validation"
)

const (
	TypePlatformApp    = "app"
	TypePlatformPlugin = "plugin"
	TypeShopwareBundle = "shopware-bundle"

	ComposerTypePlugin = "shopware-platform-plugin"
	ComposerTypeApp    = "shopware-app"
	ComposerTypeBundle = "shopware-bundle"
)

func GetExtensionByFolder(ctx context.Context, path string) (Extension, error) {
	if _, err := os.Stat(fmt.Sprintf("%s/plugin.xml", path)); err == nil {
		return nil, fmt.Errorf("shopware 5 is not supported. Please use https://github.com/FriendsOfShopware/FroshPluginUploader instead")
	}

	if _, err := os.Stat(fmt.Sprintf("%s/manifest.xml", path)); err == nil {
		return newApp(ctx, path)
	}

	if _, err := os.Stat(fmt.Sprintf("%s/composer.json", path)); err != nil {
		return nil, fmt.Errorf("unknown extension type")
	}

	var ext Extension

	ext, err := newPlatformPlugin(ctx, path)
	if err != nil {
		if errors.Is(err, ErrPlatformInvalidType) {
			ext, err = newShopwareBundle(ctx, path)
		} else {
			return nil, err
		}
	}

	return ext, err
}

func GetExtensionByZip(ctx context.Context, filePath string) (Extension, error) {
	dir, err := os.MkdirTemp("", "extension")
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	file, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, err
	}

	err = archiver.Unzip(file, dir)
	if err != nil {
		return nil, err
	}

	fileName := file.File[0].Name

	if strings.Contains(fileName, "..") {
		return nil, fmt.Errorf("invalid zip file")
	}

	extName := strings.Split(fileName, "/")[0]
	return GetExtensionByFolder(ctx, fmt.Sprintf("%s/%s", dir, extName))
}

type ExtensionTranslated struct {
	German  string `json:"german"`
	English string `json:"english"`
}

type ExtensionChangelog struct {
	German     string `json:"german"`
	English    string `json:"english"`
	Changelogs map[string]string
}

type ExtensionMetadata struct {
	Name        string
	Label       ExtensionTranslated
	Description ExtensionTranslated
}

type Extension interface {
	GetName() (string, error)
	GetComposerName() (string, error)
	// Deprecated: use the list variation instead
	GetResourcesDir() string
	GetResourcesDirs() []string

	GetIconPath() string

	// GetRootDir Returns the root folder where the code is located plugin -> src, app ->
	GetRootDir() string
	GetSourceDirs() []string
	GetVersion() (*version.Version, error)
	GetLicense() (string, error)
	GetShopwareVersionConstraint() (*version.Constraints, error)
	GetType() string
	GetPath() string
	GetChangelog() (*ExtensionChangelog, error)
	GetMetaData() *ExtensionMetadata
	UpdateMetaData(*ExtensionMetadata) error
	GetExtensionConfig() *Config
	Validate(context.Context, validation.Check)
}

// getShopwareVersionConstraintFromComposer is a shared helper for composer-based extensions
// (PlatformPlugin and ShopwareBundle) to extract the Shopware compatibility constraint from
// the composer requirements. It intentionally ignores the build-time override
// (config.Build.ShopwareVersionConstraint) so that the reported compatibility always reflects
// what the extension actually supports (e.g. for account store uploads). Use
// GetShopwareBuildVersionConstraint / GetShopwareVersionConstraintForBuild when the build-time
// override should be honored.
func getShopwareVersionConstraintFromComposer(composerRequire map[string]string) (*version.Constraints, error) {
	shopwareConstraintString, ok := composerRequire["shopware/core"]
	if !ok {
		return nil, fmt.Errorf("require.shopware/core is required")
	}

	shopwareConstraint, err := version.NewConstraint(shopwareConstraintString)
	if err != nil {
		return nil, err
	}

	return &shopwareConstraint, nil
}

// GetShopwareBuildVersionConstraint returns the Shopware version constraint configured as a
// build-time override via config.Build.ShopwareVersionConstraint. It handles a nil config
// gracefully and returns (nil, nil) when no override is configured. This is the single source
// of truth for the build-time constraint and is never mixed into an extension's reported
// compatibility constraint.
func GetShopwareBuildVersionConstraint(config *Config) (*version.Constraints, error) {
	if config != nil && config.Build.ShopwareVersionConstraint != "" {
		constraint, err := version.NewConstraint(config.Build.ShopwareVersionConstraint)
		if err != nil {
			return nil, err
		}

		return &constraint, nil
	}

	return nil, nil
}

// GetShopwareVersionConstraintForBuild resolves the constraint that should be used when building
// assets for the given extension. It honors the build-time override
// (config.Build.ShopwareVersionConstraint) when set and otherwise falls back to the extension's
// reported compatibility constraint.
func GetShopwareVersionConstraintForBuild(ext Extension) (*version.Constraints, error) {
	buildConstraint, err := GetShopwareBuildVersionConstraint(ext.GetExtensionConfig())
	if err != nil {
		return nil, err
	}

	if buildConstraint != nil {
		return buildConstraint, nil
	}

	return ext.GetShopwareVersionConstraint()
}
