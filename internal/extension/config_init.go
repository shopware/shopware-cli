package extension

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/compatibility"
)

const (
	// ConfigFileName is the canonical extension config filename.
	ConfigFileName = ".shopware-extension.yml"
	// ConfigFileNameAlt is accepted when reading existing configs.
	ConfigFileNameAlt = ".shopware-extension.yaml"

	// InitTypeApp and InitTypePlugin are the --type values for config init.
	InitTypeApp    = TypePlatformApp
	InitTypePlugin = TypePlatformPlugin
)

// InitConfigOptions controls generation of a new .shopware-extension.yml.
type InitConfigOptions struct {
	// Type is "app" or "plugin" (required for non-interactive use).
	Type string
	// Name is an optional store meta title (en).
	Name string
	// Description is an optional store description (en).
	Description string
	// Maintainer is optional free-text; written as a YAML comment only
	// (authoritative maintainer data lives in composer.json / manifest.xml).
	Maintainer string
	// Force overwrites an existing config file.
	Force bool
}

// ConfigPath returns the path of an existing config file, or "" if none.
func ConfigPath(dir string) string {
	for _, name := range []string{ConfigFileName, ConfigFileNameAlt} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// ConfigExists reports whether a .shopware-extension.yml/.yaml is present.
func ConfigExists(dir string) bool {
	return ConfigPath(dir) != ""
}

// NewDefaultConfig returns a CLI-usable default config (zip assets/composer on,
// current compatibility date).
func NewDefaultConfig() *Config {
	cfg := &Config{
		CompatibilityDate: compatibility.DefaultDate(),
		FileName:          ConfigFileName,
	}
	cfg.Build.Zip.Assets.Enabled = true
	cfg.Build.Zip.Composer.Enabled = true

	return cfg
}

// ValidateInitType checks that type is app or plugin.
func ValidateInitType(extType string) error {
	switch strings.ToLower(strings.TrimSpace(extType)) {
	case InitTypeApp, InitTypePlugin:
		return nil
	case "":
		return fmt.Errorf("extension type is required (app or plugin)")
	default:
		return fmt.Errorf("invalid extension type %q: must be %q or %q", extType, InitTypeApp, InitTypePlugin)
	}
}

// DetectInitType infers app/plugin from directory structure.
// Returns an error when the folder is not a recognizable extension.
func DetectInitType(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "manifest.xml")); err == nil {
		return InitTypeApp, nil
	}

	composerPath := filepath.Join(dir, "composer.json")
	if _, err := os.Stat(composerPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("not a Shopware extension: missing manifest.xml (app) or composer.json (plugin) in %s", dir)
		}

		return "", err
	}

	// Prefer a real plugin/bundle parse when possible; fall back to "plugin"
	// when composer.json exists (CLI still needs a config file for validation/zip).
	return InitTypePlugin, nil
}

// ValidateExtensionStructure ensures the directory matches the declared type.
func ValidateExtensionStructure(dir, extType string) error {
	if err := ValidateInitType(extType); err != nil {
		return err
	}

	switch strings.ToLower(extType) {
	case InitTypeApp:
		if _, err := os.Stat(filepath.Join(dir, "manifest.xml")); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("app extension requires manifest.xml in %s", dir)
			}

			return err
		}
	case InitTypePlugin:
		if _, err := os.Stat(filepath.Join(dir, "composer.json")); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("plugin extension requires composer.json in %s", dir)
			}

			return err
		}
	}

	return nil
}

// BuildInitConfig creates a Config from options (store metadata filled when set).
func BuildInitConfig(opts InitConfigOptions) (*Config, error) {
	if err := ValidateInitType(opts.Type); err != nil {
		return nil, err
	}

	cfg := NewDefaultConfig()

	// Apps typically ship without a composer vendor tree in the zip.
	if strings.EqualFold(opts.Type, InitTypeApp) {
		cfg.Build.Zip.Composer.Enabled = false
	}

	if opts.Name != "" {
		name := opts.Name
		cfg.Store.MetaTitle.English = &name
	}
	if opts.Description != "" {
		desc := opts.Description
		cfg.Store.Description.English = &desc
	}

	// Optional store type for plugins that are themes (not prompted separately).
	if strings.EqualFold(opts.Type, InitTypePlugin) {
		storeType := "extension"
		cfg.Store.Type = &storeType
	}

	return cfg, nil
}

// WriteConfig marshals cfg to .shopware-extension.yml under dir.
func WriteConfig(cfg *Config, dir string) error {
	return WriteConfigWithComment(cfg, dir, "")
}

// initConfigFile is a minimal YAML shape used for `extension config init` so
// zero-value bools (e.g. composer.enabled: false for apps) are always written.
// The full Config type uses omitempty on nested structs, which would drop them.
type initConfigFile struct {
	CompatibilityDate string          `yaml:"compatibility_date"`
	Store             *initStoreBlock `yaml:"store,omitempty"`
	Build             initBuildBlock  `yaml:"build"`
}

type initStoreBlock struct {
	Type        *string                  `yaml:"type,omitempty"`
	MetaTitle   ConfigTranslated[string] `yaml:"meta_title,omitempty"`
	Description ConfigTranslated[string] `yaml:"description,omitempty"`
}

type initBuildBlock struct {
	Zip initZipBlock `yaml:"zip"`
}

type initZipBlock struct {
	Assets   initToggle `yaml:"assets"`
	Composer initToggle `yaml:"composer"`
}

type initToggle struct {
	Enabled bool `yaml:"enabled"`
}

// WriteConfigWithComment writes a CLI-ready config file, optionally prefixing a comment block.
func WriteConfigWithComment(cfg *Config, dir string, comment string) error {
	outCfg := initConfigFile{
		CompatibilityDate: cfg.CompatibilityDate,
		Build: initBuildBlock{
			Zip: initZipBlock{
				Assets:   initToggle{Enabled: cfg.Build.Zip.Assets.Enabled},
				Composer: initToggle{Enabled: cfg.Build.Zip.Composer.Enabled},
			},
		},
	}

	hasStore := cfg.Store.Type != nil ||
		cfg.Store.MetaTitle.English != nil || cfg.Store.MetaTitle.German != nil ||
		cfg.Store.Description.English != nil || cfg.Store.Description.German != nil
	if hasStore {
		outCfg.Store = &initStoreBlock{
			Type:        cfg.Store.Type,
			MetaTitle:   cfg.Store.MetaTitle,
			Description: cfg.Store.Description,
		}
	}

	data, err := yaml.Marshal(outCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal extension configuration: %w", err)
	}

	var out []byte
	if comment != "" {
		for _, line := range strings.Split(strings.TrimSpace(comment), "\n") {
			out = append(out, []byte("# "+line+"\n")...)
		}
		out = append(out, '\n')
	}
	out = append(out, data...)

	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("failed to write extension configuration to %s: %w", path, err)
	}

	cfg.FileName = ConfigFileName

	return nil
}

// InitConfig validates the extension tree and writes .shopware-extension.yml.
// Returns the written file path.
func InitConfig(dir string, opts InitConfigOptions) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("cannot access extension path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("extension path must be a directory: %s", abs)
	}

	extType := strings.ToLower(strings.TrimSpace(opts.Type))
	if extType == "" {
		detected, detErr := DetectInitType(abs)
		if detErr != nil {
			return "", detErr
		}
		extType = detected
		opts.Type = extType
	}

	if err := ValidateExtensionStructure(abs, extType); err != nil {
		return "", err
	}

	if existing := ConfigPath(abs); existing != "" && !opts.Force {
		return "", fmt.Errorf("%s already exists (use --force to overwrite)", existing)
	}

	cfg, err := BuildInitConfig(opts)
	if err != nil {
		return "", err
	}

	comment := "Generated by shopware-cli extension config init"
	if opts.Maintainer != "" {
		comment += "\nMaintainer: " + opts.Maintainer
	}

	if err := WriteConfigWithComment(cfg, abs, comment); err != nil {
		return "", err
	}

	return filepath.Join(abs, ConfigFileName), nil
}
