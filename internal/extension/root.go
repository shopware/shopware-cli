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

func GetExtensionByFolder(path string) (Extension, error) {
	if _, err := os.Stat(fmt.Sprintf("%s/plugin.xml", path)); err == nil {
		return nil, fmt.Errorf("shopware 5 is not supported. Please use https://github.com/FriendsOfShopware/FroshPluginUploader instead")
	}

	if _, err := os.Stat(fmt.Sprintf("%s/manifest.xml", path)); err == nil {
		return newApp(path)
	}

	if _, err := os.Stat(fmt.Sprintf("%s/composer.json", path)); err != nil {
		return nil, fmt.Errorf("unknown extension type")
	}

	var ext Extension

	ext, err := newPlatformPlugin(path)
	if err != nil {
		if errors.Is(err, ErrPlatformInvalidType) {
			ext, err = newShopwareBundle(path)
		} else {
			return nil, err
		}
	}

	return ext, err
}

func GetExtensionByZip(filePath string) (Extension, error) {
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

	err = Unzip(file, dir)
	if err != nil {
		return nil, err
	}

	fileName := file.File[0].Name

	if strings.Contains(fileName, "..") {
		return nil, fmt.Errorf("invalid zip file")
	}

	extName := strings.Split(fileName, "/")[0]
	return GetExtensionByFolder(fmt.Sprintf("%s/%s", dir, extName))
}

type extensionTranslated struct {
	German  string `json:"german"`
	English string `json:"english"`
}

type ExtensionChangelog struct {
	German     string `json:"german"`
	English    string `json:"english"`
	Changelogs map[string]string
}

type extensionMetadata struct {
	Name        string
	Label       extensionTranslated
	Description extensionTranslated
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
	GetMetaData() *extensionMetadata
	GetExtensionConfig() *Config
	Validate(context.Context, validation.Check)
}
