package extension

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/changelog"

	"gopkg.in/yaml.v3"
)

type ConfigBuild struct {
	ExtraBundles              []ConfigExtraBundle `yaml:"extraBundles,omitempty"`
	ShopwareVersionConstraint string              `yaml:"shopwareVersionConstraint,omitempty"`
	Zip                       struct {
		Composer struct {
			Enabled          bool     `yaml:"enabled"`
			BeforeHooks      []string `yaml:"before_hooks,omitempty"`
			AfterHooks       []string `yaml:"after_hooks,omitempty"`
			ExcludedPackages []string `yaml:"excluded_packages,omitempty"`
		} `yaml:"composer"`
		Assets struct {
			Enabled                    bool     `yaml:"enabled"`
			BeforeHooks                []string `yaml:"before_hooks,omitempty"`
			AfterHooks                 []string `yaml:"after_hooks,omitempty"`
			EnableESBuildForAdmin      bool     `yaml:"enable_es_build_for_admin"`
			EnableESBuildForStorefront bool     `yaml:"enable_es_build_for_storefront"`
			DisableSass                bool     `yaml:"es_build_disable_sass"`
			NpmStrict                  bool     `yaml:"npm_strict"`
		} `yaml:"assets"`
		Pack struct {
			Excludes struct {
				Paths []string `yaml:"paths,omitempty"`
			} `yaml:"excludes,omitempty"`
			BeforeHooks []string `yaml:"before_hooks,omitempty"`
		} `yaml:"pack"`
	} `yaml:"zip"`
}

type ConfigExtraBundle struct {
	Path string `yaml:"path"`
	Name string `yaml:"name"`
}

type ConfigStore struct {
	Availabilities                      *[]string                          `yaml:"availabilities"`
	DefaultLocale                       *string                            `yaml:"default_locale"`
	Localizations                       *[]string                          `yaml:"localizations"`
	Categories                          *[]string                          `yaml:"categories"`
	Type                                *string                            `yaml:"type"`
	Icon                                *string                            `yaml:"icon"`
	AutomaticBugfixVersionCompatibility *bool                              `yaml:"automatic_bugfix_version_compatibility"`
	Description                         ConfigTranslated[string]           `yaml:"description"`
	InstallationManual                  ConfigTranslated[string]           `yaml:"installation_manual"`
	Tags                                ConfigTranslated[[]string]         `yaml:"tags"`
	Videos                              ConfigTranslated[[]string]         `yaml:"videos"`
	Highlights                          ConfigTranslated[[]string]         `yaml:"highlights"`
	Features                            ConfigTranslated[[]string]         `yaml:"features"`
	Faq                                 ConfigTranslated[[]ConfigStoreFaq] `yaml:"faq"`
	Images                              *[]ConfigStoreImage                `yaml:"images,omitempty"`
	ImageDirectory                      *string                            `yaml:"image_directory,omitempty"`
}

type Translatable interface {
	string | []string | []ConfigStoreFaq
}

type ConfigTranslated[T Translatable] struct {
	German  *T `yaml:"de"`
	English *T `yaml:"en"`
}

type ConfigStoreFaq struct {
	Question string `yaml:"question"`
	Answer   string `yaml:"answer"`
}

type ConfigStoreImage struct {
	File     string                   `yaml:"file"`
	Activate ConfigStoreImageActivate `yaml:"activate"`
	Preview  ConfigStoreImagePreview  `yaml:"preview"`
	Priority int                      `yaml:"priority"`
}

type ConfigStoreImageActivate struct {
	German  bool `yaml:"de"`
	English bool `yaml:"en"`
}

type ConfigStoreImagePreview struct {
	German  bool `yaml:"de"`
	English bool `yaml:"en"`
}

type Config struct {
	FileName  string
	Store     ConfigStore      `yaml:"store"`
	Build     ConfigBuild      `yaml:"build"`
	Changelog changelog.Config `yaml:"changelog"`
}

func readExtensionConfig(dir string) (*Config, error) {
	config := &Config{}
	config.Build.Zip.Assets.Enabled = true
	config.Build.Zip.Composer.Enabled = true
	config.FileName = ".shopware-extension.yml"

	configLocation := ""

	if _, err := os.Stat(filepath.Join(dir, ".shopware-extension.yml")); err == nil {
		configLocation = filepath.Join(dir, ".shopware-extension.yml")
	} else if _, err := os.Stat(filepath.Join(dir, ".shopware-extension.yaml")); err == nil {
		configLocation = filepath.Join(dir, ".shopware-extension.yaml")
	} else {
		return config, nil
	}

	errorFormat := "file: " + configLocation + ": %v"

	fileHandle, err := os.ReadFile(configLocation)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	err = yaml.Unmarshal(fileHandle, &config)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	config.FileName = filepath.Base(configLocation)

	err = validateExtensionConfig(config)
	if err != nil {
		return nil, fmt.Errorf(errorFormat, err)
	}

	return config, nil
}

func validateExtensionConfig(config *Config) error {
	if config.Store.Tags.English != nil && len(*config.Store.Tags.English) > 5 {
		return fmt.Errorf("store.info.tags.en can contain maximal 5 items")
	}

	if config.Store.Tags.German != nil && len(*config.Store.Tags.German) > 5 {
		return fmt.Errorf("store.info.tags.de can contain maximal 5 items")
	}

	if config.Store.Videos.English != nil && len(*config.Store.Videos.English) > 2 {
		return fmt.Errorf("store.info.videos.en can contain maximal 2 items")
	}

	if config.Store.Videos.German != nil && len(*config.Store.Videos.German) > 2 {
		return fmt.Errorf("store.info.videos.de can contain maximal 2 items")
	}

	return nil
}
