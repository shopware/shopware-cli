package extension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/validation"
)

var ErrPlatformInvalidType = errors.New("invalid composer type")

type PlatformPlugin struct {
	path     string
	Composer PlatformComposerJson
	config   *Config
}

// GetRootDir returns the src directory of the plugin.
func (p PlatformPlugin) GetRootDir() string {
	return filepath.Join(p.path, "src")
}

func (p PlatformPlugin) GetSourceDirs() []string {
	var result []string

	for _, val := range p.Composer.Autoload.Psr4 {
		result = append(result, filepath.Join(p.path, val))
	}

	return result
}

// GetResourcesDir returns the resources directory of the plugin.
func (p PlatformPlugin) GetResourcesDir() string {
	return filepath.Join(p.GetRootDir(), "Resources")
}

func (p PlatformPlugin) GetResourcesDirs() []string {
	var result []string

	for _, val := range p.GetSourceDirs() {
		result = append(result, filepath.Join(val, "Resources"))
	}

	return result
}

func newPlatformPlugin(ctx context.Context, path string) (*PlatformPlugin, error) {
	composerJsonFile := path + "/composer.json"
	if _, err := os.Stat(composerJsonFile); err != nil {
		return nil, err
	}

	jsonFile, err := os.ReadFile(composerJsonFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read composer.json: %w", err)
	}

	var composerJson PlatformComposerJson
	if err := json.Unmarshal(jsonFile, &composerJson); err != nil {
		return nil, fmt.Errorf("cannot parse composer.json: %w", err)
	}

	if composerJson.Type != ComposerTypePlugin {
		return nil, ErrPlatformInvalidType
	}

	cfg, err := readExtensionConfig(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("cannot read extension config: %w", err)
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
		return "", errors.New("extension name is empty")
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
	return getShopwareVersionConstraintFromComposer(p.Composer.Require)
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

func (p PlatformPlugin) GetMetaData() *ExtensionMetadata {
	return &ExtensionMetadata{
		Name: p.Composer.Name,
		Label: ExtensionTranslated{
			German:  p.Composer.Extra.Label["de-DE"],
			English: p.Composer.Extra.Label["en-GB"],
		},
		Description: ExtensionTranslated{
			German:  p.Composer.Extra.Description["de-DE"],
			English: p.Composer.Extra.Description["en-GB"],
		},
	}
}

func (p PlatformPlugin) UpdateMetaData(metadata *ExtensionMetadata) error {
	composerJsonFile := p.path + "/composer.json"

	composerJson, err := os.ReadFile(composerJsonFile)
	if err != nil {
		return fmt.Errorf("could not read composer.json: %w", err)
	}

	var composerJsonStruct map[string]interface{}
	if err := json.Unmarshal(composerJson, &composerJsonStruct); err != nil {
		return fmt.Errorf("could not unmarshal composer.json: %w", err)
	}

	extra, ok := composerJsonStruct["extra"].(map[string]interface{})
	if !ok {
		extra = make(map[string]interface{})
		composerJsonStruct["extra"] = extra
	}

	changed := false

	if metadata.Label.German != "" || metadata.Label.English != "" {
		label, ok := extra["label"].(map[string]interface{})
		if !ok {
			label = make(map[string]interface{})
		}
		if metadata.Label.German != "" {
			label["de-DE"] = metadata.Label.German
		}
		if metadata.Label.English != "" {
			label["en-GB"] = metadata.Label.English
		}
		extra["label"] = label
		changed = true
	}

	if metadata.Description.German != "" || metadata.Description.English != "" {
		description, ok := extra["description"].(map[string]interface{})
		if !ok {
			description = make(map[string]interface{})
		}
		if metadata.Description.German != "" {
			description["de-DE"] = metadata.Description.German
		}
		if metadata.Description.English != "" {
			description["en-GB"] = metadata.Description.English
		}
		extra["description"] = description
		changed = true
	}

	if !changed {
		return nil
	}

	newComposerJson, err := json.MarshalIndent(composerJsonStruct, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal composer.json: %w", err)
	}

	newComposerJson = append(newComposerJson, '\n')

	if err := os.WriteFile(composerJsonFile, newComposerJson, 0o644); err != nil {
		return fmt.Errorf("could not write composer.json: %w", err)
	}

	return nil
}

func (p PlatformPlugin) GetIconPath() string {
	pluginIcon := p.Composer.Extra.PluginIcon

	if pluginIcon == "" {
		pluginIcon = "src/Resources/config/plugin.png"
	}

	return filepath.Join(p.path, pluginIcon)
}

func (p PlatformPlugin) Validate(_ context.Context, check validation.Check) {
	if p.Composer.Name == "" {
		check.AddResult(validation.CheckResult{
			Path:       "composer.json",
			Identifier: "metadata.name",
			Message:    "Key `name` is required",
			Severity:   validation.SeverityError,
		})
	}

	if p.Composer.Type == "" {
		check.AddResult(validation.CheckResult{
			Path:       "composer.json",
			Identifier: "metadata.type",
			Message:    "Key `type` is required",
			Severity:   validation.SeverityError,
		})
	} else if p.Composer.Type != ComposerTypePlugin {
		check.AddResult(validation.CheckResult{
			Path:       "composer.json",
			Identifier: "metadata.type",
			Message:    "The composer type must be shopware-platform-plugin",
			Severity:   validation.SeverityError,
		})
	}

	if p.Composer.Description == "" {
		check.AddResult(validation.CheckResult{
			Path:       "composer.json",
			Identifier: "metadata.description.required",
			Message:    "Key `description` is required",
			Severity:   validation.SeverityError,
		})
	}

	if p.Composer.License == "" {
		check.AddResult(validation.CheckResult{
			Path:       "composer.json",
			Identifier: "metadata.license",
			Message:    "Key `license` is required",
			Severity:   validation.SeverityError,
		})
	}

	if p.Composer.Version == "" {
		check.AddResult(validation.CheckResult{
			Path:       "composer.json",
			Identifier: "metadata.version",
			Message:    "Key `version` is required",
			Severity:   validation.SeverityError,
		})
	}

	if len(p.Composer.Authors) == 0 {
		check.AddResult(validation.CheckResult{
			Path:       "composer.json",
			Identifier: "metadata.author",
			Message:    "Key `authors` is required",
			Severity:   validation.SeverityError,
		})
	}

	if len(p.Composer.Require) == 0 {
		check.AddResult(validation.CheckResult{
			Path:       "composer.json",
			Identifier: "metadata.require",
			Message:    "Key `require` is required",
			Severity:   validation.SeverityError,
		})
	} else {
		_, exists := p.Composer.Require["shopware/core"]

		if !exists {
			check.AddResult(validation.CheckResult{
				Path:       "composer.json",
				Identifier: "metadata.require",
				Message:    "You need to require \"shopware/core\" package",
				Severity:   validation.SeverityError,
			})
		}
	}

	requiredKeys := []string{"de-DE", "en-GB"}

	if !p.GetExtensionConfig().Store.IsInGermanStore() {
		requiredKeys = []string{"en-GB"}
	}

	for _, key := range requiredKeys {
		_, hasLabel := p.Composer.Extra.Label[key]
		_, hasDescription := p.Composer.Extra.Description[key]
		_, hasManufacturer := p.Composer.Extra.ManufacturerLink[key]
		_, hasSupportLink := p.Composer.Extra.SupportLink[key]

		if !hasLabel {
			check.AddResult(validation.CheckResult{
				Path:       "composer.json",
				Identifier: "metadata.label.translation." + key,
				Message:    fmt.Sprintf("extra.label for language %s is required", key),
				Severity:   validation.SeverityError,
			})
		}

		if !hasDescription {
			check.AddResult(validation.CheckResult{
				Path:       "composer.json",
				Identifier: "metadata.description.translation." + key,
				Message:    fmt.Sprintf("extra.description for language %s is required", key),
				Severity:   validation.SeverityError,
			})
		}

		if !hasManufacturer {
			check.AddResult(validation.CheckResult{
				Path:       "composer.json",
				Identifier: "metadata.manufacturer",
				Message:    fmt.Sprintf("extra.manufacturerLink for language %s is required", key),
				Severity:   validation.SeverityError,
			})
		}

		if !hasSupportLink {
			check.AddResult(validation.CheckResult{
				Path:       "composer.json",
				Identifier: "metadata.support",
				Message:    fmt.Sprintf("extra.supportLink for language %s is required", key),
				Severity:   validation.SeverityError,
			})
		}
	}

	if len(p.Composer.Autoload.Psr0) == 0 && len(p.Composer.Autoload.Psr4) == 0 {
		check.AddResult(validation.CheckResult{
			Path:       "composer.json",
			Identifier: "metadata.autoload",
			Message:    "At least one of the properties psr-0 or psr-4 are required in the composer.json",
			Severity:   validation.SeverityError,
		})
	}

	validateExtensionIcon(p, check)

	validateTheme(p, check)
}
