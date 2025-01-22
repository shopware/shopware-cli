package extension

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/shopware/shopware-cli/version"
)

type App struct {
	path     string
	manifest Manifest
	config   *Config
}

func (a App) GetRootDir() string {
	return a.path
}

func (a App) GetResourcesDir() string {
	return path.Join(a.path, "Resources")
}

func newApp(path string) (*App, error) {
	appFileName := fmt.Sprintf("%s/manifest.xml", path)

	if _, err := os.Stat(appFileName); err != nil {
		return nil, err
	}

	appFile, err := os.ReadFile(appFileName)
	if err != nil {
		return nil, fmt.Errorf("newApp: %v", err)
	}

	var manifest Manifest
	err = xml.Unmarshal(appFile, &manifest)
	if err != nil {
		return nil, fmt.Errorf("newApp: %v", err)
	}

	cfg, err := readExtensionConfig(path)
	if err != nil {
		return nil, fmt.Errorf("newApp: %v", err)
	}

	app := App{
		path:     path,
		manifest: manifest,
		config:   cfg,
	}

	return &app, nil
}

func (a App) GetName() (string, error) {
	return a.manifest.Meta.Name, nil
}

func (a App) GetVersion() (*version.Version, error) {
	return version.NewVersion(a.manifest.Meta.Version)
}

func (a App) GetLicense() (string, error) {
	return a.manifest.Meta.License, nil
}

func (a App) GetExtensionConfig() *Config {
	return a.config
}

func (a App) GetShopwareVersionConstraint() (*version.Constraints, error) {
	if a.config != nil && a.config.Build.ShopwareVersionConstraint != "" {
		v, err := version.NewConstraint(a.config.Build.ShopwareVersionConstraint)
		if err != nil {
			return nil, err
		}

		return &v, err
	}

	if a.manifest.Meta.Compatibility != "" {
		v, err := version.NewConstraint(a.manifest.Meta.Compatibility)
		if err != nil {
			return nil, err
		}

		return &v, nil
	}

	v, err := version.NewConstraint("~6.4")
	if err != nil {
		return nil, err
	}

	return &v, err
}

func (App) GetType() string {
	return TypePlatformApp
}

func (a App) GetPath() string {
	return a.path
}

func (a App) GetChangelog() (*ExtensionChangelog, error) {
	return parseExtensionMarkdownChangelog(a)
}

func (a App) GetMetaData() *extensionMetadata {
	german := []string{"de-DE", "de"}
	english := []string{"en-GB", "en-US", "en", ""}

	return &extensionMetadata{
		Label: extensionTranslated{
			German:  a.manifest.Meta.Label.GetValueByLanguage(german),
			English: a.manifest.Meta.Label.GetValueByLanguage(english),
		},
		Description: extensionTranslated{
			German:  a.manifest.Meta.Description.GetValueByLanguage(german),
			English: a.manifest.Meta.Description.GetValueByLanguage(english),
		},
	}
}

func (a App) Validate(_ context.Context, ctx *ValidationContext) {
	validateTheme(ctx)

	appIcon := a.manifest.Meta.Icon

	if appIcon == "" {
		appIcon = "Resources/config/plugin.png"
	}

	if _, err := os.Stat(filepath.Join(a.GetPath(), appIcon)); os.IsNotExist(err) {
		ctx.AddError(fmt.Sprintf("Cannot find app icon at %s", appIcon))
	}
}
