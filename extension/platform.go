package extension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/phplint"
	"github.com/shopware/shopware-cli/logging"
)

var ErrPlatformInvalidType = errors.New("invalid composer type")

type PlatformPlugin struct {
	path     string
	Composer PlatformComposerJson
	config   *Config
}

// GetRootDir returns the src directory of the plugin.
func (p PlatformPlugin) GetRootDir() string {
	return path.Join(p.path, "src")
}

func (p PlatformPlugin) GetSourceDirs() []string {
	var result []string

	for _, val := range p.Composer.Autoload.Psr4 {
		result = append(result, path.Join(p.path, val))
	}

	return result
}

// GetResourcesDir returns the resources directory of the plugin.
func (p PlatformPlugin) GetResourcesDir() string {
	return path.Join(p.GetRootDir(), "Resources")
}

func (p PlatformPlugin) GetResourcesDirs() []string {
	var result []string

	for _, val := range p.GetSourceDirs() {
		result = append(result, path.Join(val, "Resources"))
	}

	return result
}

func newPlatformPlugin(path string) (*PlatformPlugin, error) {
	composerJsonFile := fmt.Sprintf("%s/composer.json", path)
	if _, err := os.Stat(composerJsonFile); err != nil {
		return nil, err
	}

	jsonFile, err := os.ReadFile(composerJsonFile)
	if err != nil {
		return nil, fmt.Errorf("newPlatformPlugin: %v", err)
	}

	var composerJson PlatformComposerJson
	if err := json.Unmarshal(jsonFile, &composerJson); err != nil {
		return nil, fmt.Errorf("newPlatformPlugin: %v", err)
	}

	if composerJson.Type != ComposerTypePlugin {
		return nil, ErrPlatformInvalidType
	}

	cfg, err := readExtensionConfig(path)
	if err != nil {
		return nil, fmt.Errorf("newPlatformPlugin: %v", err)
	}

	extension := PlatformPlugin{
		Composer: composerJson,
		path:     path,
		config:   cfg,
	}

	return &extension, nil
}

type PlatformComposerJson struct {
	Name        string   `json:"name"`
	Keywords    []string `json:"keywords"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Type        string   `json:"type"`
	License     string   `json:"license"`
	Authors     []struct {
		Name     string `json:"name"`
		Homepage string `json:"homepage"`
	} `json:"authors"`
	Require  map[string]string         `json:"require"`
	Extra    platformComposerJsonExtra `json:"extra"`
	Autoload struct {
		Psr0 map[string]string `json:"psr-0"`
		Psr4 map[string]string `json:"psr-4"`
	} `json:"autoload"`
	Suggest map[string]string `json:"suggest"`
}

type platformComposerJsonExtra struct {
	ShopwarePluginClass string            `json:"shopware-plugin-class"`
	Label               map[string]string `json:"label"`
	Description         map[string]string `json:"description"`
	ManufacturerLink    map[string]string `json:"manufacturerLink"`
	SupportLink         map[string]string `json:"supportLink"`
	PluginIcon          string            `json:"plugin-icon"`
}

func (p PlatformPlugin) GetName() (string, error) {
	if p.Composer.Extra.ShopwarePluginClass == "" {
		return "", fmt.Errorf("extension name is empty")
	}

	parts := strings.Split(p.Composer.Extra.ShopwarePluginClass, "\\")

	return parts[len(parts)-1], nil
}

func (p PlatformPlugin) GetComposerName() (string, error) {
	return p.Composer.Name, nil
}

func (p PlatformPlugin) GetExtensionConfig() *Config {
	return p.config
}

func (p PlatformPlugin) GetShopwareVersionConstraint() (*version.Constraints, error) {
	if p.config != nil && p.config.Build.ShopwareVersionConstraint != "" {
		constraint, err := version.NewConstraint(p.config.Build.ShopwareVersionConstraint)
		if err != nil {
			return nil, err
		}

		return &constraint, nil
	}

	shopwareConstraintString, ok := p.Composer.Require["shopware/core"]

	if !ok {
		return nil, fmt.Errorf("require.shopware/core is required")
	}

	shopwareConstraint, err := version.NewConstraint(shopwareConstraintString)
	if err != nil {
		return nil, err
	}

	return &shopwareConstraint, err
}

func (PlatformPlugin) GetType() string {
	return TypePlatformPlugin
}

func (p PlatformPlugin) GetVersion() (*version.Version, error) {
	return version.NewVersion(p.Composer.Version)
}

func (p PlatformPlugin) GetChangelog() (*ExtensionChangelog, error) {
	return parseExtensionMarkdownChangelog(p)
}

func (p PlatformPlugin) GetLicense() (string, error) {
	return p.Composer.License, nil
}

func (p PlatformPlugin) GetPath() string {
	return p.path
}

func (p PlatformPlugin) GetMetaData() *extensionMetadata {
	return &extensionMetadata{
		Name: p.Composer.Name,
		Label: extensionTranslated{
			German:  p.Composer.Extra.Label["de-DE"],
			English: p.Composer.Extra.Label["en-GB"],
		},
		Description: extensionTranslated{
			German:  p.Composer.Extra.Description["de-DE"],
			English: p.Composer.Extra.Description["en-GB"],
		},
	}
}

func (p PlatformPlugin) Validate(c context.Context, ctx *ValidationContext) {
	if p.Composer.Name == "" {
		ctx.AddError("metadata.name", "Key `name` is required")
	}

	if p.Composer.Type == "" {
		ctx.AddError("metadata.type", "Key `type` is required")
	} else if p.Composer.Type != ComposerTypePlugin {
		ctx.AddError("metadata.type", "The composer type must be shopware-platform-plugin")
	}

	if p.Composer.Description == "" {
		ctx.AddError("metadata.description", "Key `description` is required")
	}

	if p.Composer.License == "" {
		ctx.AddError("metadata.license", "Key `license` is required")
	}

	if p.Composer.Version == "" {
		ctx.AddError("metadata.version", "Key `version` is required")
	}

	if len(p.Composer.Authors) == 0 {
		ctx.AddError("metadata.author", "Key `authors` is required")
	}

	if len(p.Composer.Require) == 0 {
		ctx.AddError("metadata.require", "Key `require` is required")
	} else {
		_, exists := p.Composer.Require["shopware/core"]

		if !exists {
			ctx.AddError("metadata.require", "You need to require \"shopware/core\" package")
		}
	}

	requiredKeys := []string{"de-DE", "en-GB"}

	for _, key := range requiredKeys {
		_, hasLabel := p.Composer.Extra.Label[key]
		_, hasDescription := p.Composer.Extra.Description[key]
		_, hasManufacturer := p.Composer.Extra.ManufacturerLink[key]
		_, hasSupportLink := p.Composer.Extra.SupportLink[key]

		if !hasLabel {
			ctx.AddError("metadata.label", fmt.Sprintf("extra.label for language %s is required", key))
		}

		if !hasDescription {
			ctx.AddError("metadata.description", fmt.Sprintf("extra.description for language %s is required", key))
		}

		if !hasManufacturer {
			ctx.AddError("metadata.manufacturer", fmt.Sprintf("extra.manufacturerLink for language %s is required", key))
		}

		if !hasSupportLink {
			ctx.AddError("metadata.support", fmt.Sprintf("extra.supportLink for language %s is required", key))
		}
	}

	if len(p.Composer.Autoload.Psr0) == 0 && len(p.Composer.Autoload.Psr4) == 0 {
		ctx.AddError("metadata.autoload", "At least one of the properties psr-0 or psr-4 are required in the composer.json")
	}

	pluginIcon := p.Composer.Extra.PluginIcon

	if pluginIcon == "" {
		pluginIcon = "src/Resources/config/plugin.png"
	}

	// check if the plugin icon exists
	if _, err := os.Stat(filepath.Join(p.GetPath(), pluginIcon)); os.IsNotExist(err) {
		ctx.AddError("metadata.icon", fmt.Sprintf("The plugin icon %s does not exist", pluginIcon))
	}

	validateTheme(ctx)
	validatePHPFiles(c, ctx)
}

func validatePHPFiles(c context.Context, ctx *ValidationContext) {
	constraint, err := ctx.Extension.GetShopwareVersionConstraint()
	if err != nil {
		ctx.AddError("php.linter", fmt.Sprintf("Could not parse shopware version constraint: %s", err.Error()))
		return
	}

	phpVersion, err := GetPhpVersion(c, constraint)
	if err != nil {
		ctx.AddWarning("php.linter", fmt.Sprintf("Could not find min php version for plugin: %s", err.Error()))
		return
	}

	if phpVersion == "7.2" {
		phpVersion = "7.3"
		logging.FromContext(c).Infof("PHP 7.2 is not supported for PHP linting, using 7.3 now")
	}

	for _, val := range ctx.Extension.GetSourceDirs() {
		phpErrors, err := phplint.LintFolder(c, phpVersion, val)

		if err != nil {
			ctx.AddWarning("php.linter", fmt.Sprintf("Could not lint php files: %s", err.Error()))
			continue
		}

		for _, error := range phpErrors {
			ctx.AddError("php.linter", fmt.Sprintf("%s: %s", error.File, error.Message))
		}
	}
}

func GetPhpVersion(ctx context.Context, constraint *version.Constraints) (string, error) {
	r, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://raw.githubusercontent.com/FriendsOfShopware/shopware-static-data/main/data/php-version.json", http.NoBody)

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return "", err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("GetPhpVersion: %v", err)
		}
	}()

	var shopwareToPHPVersion map[string]string

	err = json.NewDecoder(resp.Body).Decode(&shopwareToPHPVersion)
	if err != nil {
		return "", err
	}

	for shopwareVersion, phpVersion := range shopwareToPHPVersion {
		shopwareVersionConstraint, err := version.NewVersion(shopwareVersion)
		if err != nil {
			continue
		}

		if constraint.Check(shopwareVersionConstraint) {
			return phpVersion, nil
		}
	}

	return "", errors.New("could not find php version for shopware version")
}
