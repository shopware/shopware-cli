package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/validation"
)

type ShopwareBundle struct {
	path     string
	Composer shopwareBundleComposerJson
	config   *Config
}

func newShopwareBundle(ctx context.Context, path string) (*ShopwareBundle, error) {
	composerJsonFile := fmt.Sprintf("%s/composer.json", path)
	if _, err := os.Stat(composerJsonFile); err != nil {
		return nil, err
	}

	jsonFile, err := os.ReadFile(composerJsonFile)
	if err != nil {
		return nil, fmt.Errorf("newShopwareBundle: %v", err)
	}

	var composerJson shopwareBundleComposerJson
	err = json.Unmarshal(jsonFile, &composerJson)
	if err != nil {
		return nil, fmt.Errorf("newShopwareBundle: %v", err)
	}

	if composerJson.Type != "shopware-bundle" {
		return nil, fmt.Errorf("newShopwareBundle: composer.json type is not shopware-bundle")
	}

	if composerJson.Extra.BundleName == "" {
		return nil, fmt.Errorf("composer.json does not contain shopware-bundle-name in extra")
	}

	cfg, err := readExtensionConfig(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("newShopwareBundle: %v", err)
	}

	extension := ShopwareBundle{
		Composer: composerJson,
		path:     path,
		config:   cfg,
	}

	return &extension, nil
}

type composerAutoload struct {
	Psr4 map[string]string `json:"psr-4"`
}

type shopwareBundleComposerJson struct {
	Name     string                          `json:"name"`
	Type     string                          `json:"type"`
	License  string                          `json:"license"`
	Version  string                          `json:"version"`
	Require  map[string]string               `json:"require"`
	Extra    shopwareBundleComposerJsonExtra `json:"extra"`
	Suggest  map[string]string               `json:"suggest"`
	Autoload composerAutoload                `json:"autoload"`
}

type shopwareBundleComposerJsonExtra struct {
	BundleName string `json:"shopware-bundle-name"`
}

func (p ShopwareBundle) GetComposerName() (string, error) {
	return p.Composer.Name, nil
}

// GetRootDir returns the src directory of the bundle.
func (p ShopwareBundle) GetRootDir() string {
	return filepath.Join(p.path, "src")
}

func (p ShopwareBundle) GetSourceDirs() []string {
	var result []string

	for _, val := range p.Composer.Autoload.Psr4 {
		result = append(result, filepath.Join(p.path, val))
	}

	return result
}

// GetResourcesDir returns the resources directory of the shopware bundle.
func (p ShopwareBundle) GetResourcesDir() string {
	return filepath.Join(p.GetRootDir(), "Resources")
}

func (p ShopwareBundle) GetResourcesDirs() []string {
	var result []string

	for _, val := range p.GetSourceDirs() {
		result = append(result, filepath.Join(val, "Resources"))
	}

	return result
}

func (p ShopwareBundle) GetName() (string, error) {
	return p.Composer.Extra.BundleName, nil
}

func (p ShopwareBundle) GetExtensionConfig() *Config {
	return p.config
}

func (p ShopwareBundle) GetShopwareVersionConstraint() (*version.Constraints, error) {
	return getShopwareVersionConstraintFromComposer(p.config, p.Composer.Require)
}

func (ShopwareBundle) GetType() string {
	return TypeShopwareBundle
}

func (p ShopwareBundle) GetVersion() (*version.Version, error) {
	return version.NewVersion(p.Composer.Version)
}

func (p ShopwareBundle) GetChangelog() (*ExtensionChangelog, error) {
	return parseExtensionMarkdownChangelog(p)
}

func (p ShopwareBundle) GetLicense() (string, error) {
	return p.Composer.License, nil
}

func (p ShopwareBundle) GetPath() string {
	return p.path
}

func (p ShopwareBundle) GetIconPath() string {
	return ""
}

func (p ShopwareBundle) GetMetaData() *extensionMetadata {
	return &extensionMetadata{
		Label: extensionTranslated{
			German:  "FALLBACK",
			English: "FALLBACK",
		},
		Description: extensionTranslated{
			German:  "FALLBACK",
			English: "FALLBACK",
		},
	}
}

func (p ShopwareBundle) Validate(c context.Context, check validation.Check) {
	// ShopwareBundle validation is currently empty but signature updated to match interface
}
