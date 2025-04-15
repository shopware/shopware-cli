package extension

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/shyim/go-version"
)

type App struct {
	path     string
	manifest Manifest
	config   *Config
}

func (a App) GetRootDir() string {
	return a.path
}

func (a App) GetSourceDirs() []string {
	return []string{a.path}
}

func (a App) GetResourcesDir() string {
	return path.Join(a.path, "Resources")
}

func (a App) GetResourcesDirs() []string {
	return []string{path.Join(a.path, "Resources")}
}

func (a App) GetComposerName() (string, error) {
	return "", fmt.Errorf("app does not have a composer name")
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
		ctx.AddError("metadata.icon", fmt.Sprintf("Cannot find app icon at %s", appIcon))
	}

	allowedTwigLocations := []string{path.Join(a.GetRootDir(), "Resources", "views"), path.Join(a.GetRootDir(), "Resources", "scripts")}

	_ = filepath.Walk(a.GetRootDir(), func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == ".php" {
			ctx.AddError("zip.disallowed_php_file", fmt.Sprintf("Found unexpected PHP file %s, PHP files are not allowed in Apps", path))
		}

		if filepath.Ext(path) == ".twig" && (!strings.HasPrefix(path, allowedTwigLocations[0]) && !strings.HasPrefix(path, allowedTwigLocations[1])) {
			ctx.AddError("zip.disallowed_twig_file", fmt.Sprintf("Found unexpected Twig file %s. Twig files should be at Resources/views or Resources/scripts", path))
		}

		return nil
	})

	if a.manifest.Meta.Author == "" {
		ctx.AddError("metadata.author", "The element meta:author was not found in the manifest.xml")
	}

	if a.manifest.Meta.Copyright == "" {
		ctx.AddError("metadata.copyright", "The element meta:copyright was not found in the manifest.xml")
	}

	if a.manifest.Meta.License == "" {
		ctx.AddError("metadata.license", "The element meta:license was not found in the manifest.xml")
	}

	if a.manifest.Setup != nil && a.manifest.Setup.Secret != "" {
		ctx.AddError("metadata.setup", "The xml element setup:secret is only for local development, please remove it. You can find your generated app secret on your extension detail page in the master data section. For more information see https://docs.shopware.com/en/shopware-platform-dev-en/app-system-guide/setup#authorisation")
	}
}
