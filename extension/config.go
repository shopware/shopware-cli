package extension

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/invopop/jsonschema"
	orderedmap "github.com/wk8/go-ordered-map/v2"
	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/changelog"
)

type ConfigBuild struct {
	// ExtraBundles can be used to declare additional bundles to be considered for building
	ExtraBundles []ConfigExtraBundle `yaml:"extraBundles,omitempty"`
	// Override the shopware version constraint for building, can be used to specify the version of the shopware to use for building
	ShopwareVersionConstraint string `yaml:"shopwareVersionConstraint,omitempty"`
	// Configuration for zipping
	Zip ConfigBuildZip `yaml:"zip"`
}

// Configuration for zipping.
type ConfigBuildZip struct {
	// Configuration for composer
	Composer ConfigBuildZipComposer `yaml:"composer,omitempty"`
	// Configuration for assets
	Assets ConfigBuildZipAssets `yaml:"assets,omitempty"`
	// Configuration for packing
	Pack ConfigBuildZipPack `yaml:"pack,omitempty"`

	Checksum ConfigBuildZipChecksum `yaml:"checksum,omitempty"`
}

// Configuration for checksum calculation.
type ConfigBuildZipChecksum struct {
	// Following files will be excluded from the checksum calculation
	Ignore []string `yaml:"ignore,omitempty"`
}

type ConfigBuildZipComposer struct {
	// When enabled, a vendor folder will be created in the zip build
	Enabled bool `yaml:"enabled"`
	// Commands to run before the composer install
	BeforeHooks []string `yaml:"before_hooks,omitempty"`
	// Commands to run after the composer install
	AfterHooks []string `yaml:"after_hooks,omitempty"`
	// Composer packages to be excluded from the zip build
	ExcludedPackages []string `yaml:"excluded_packages,omitempty"`
}

type ConfigBuildZipAssets struct {
	// When enabled, the shopware-cli build the assets
	Enabled bool `yaml:"enabled"`
	// Commands to run before the assets build
	BeforeHooks []string `yaml:"before_hooks,omitempty"`
	// Commands to run after the assets build
	AfterHooks []string `yaml:"after_hooks,omitempty"`
	// When enabled, builtin esbuild will be used for the admin assets
	EnableESBuildForAdmin bool `yaml:"enable_es_build_for_admin"`
	// When enabled, builtin esbuild will be used for the storefront assets
	EnableESBuildForStorefront bool `yaml:"enable_es_build_for_storefront"`
	// When disabled, builtin sass support will be disabled
	DisableSass bool `yaml:"disable_sass"`
	// When enabled, npm will install only production dependencies
	NpmStrict bool `yaml:"npm_strict"`
}

type ConfigBuildZipPackExcludes struct {
	// Paths to exclude from the zip build
	Paths []string `yaml:"paths,omitempty"`
}

type ConfigBuildZipPack struct {
	// Excludes can be used to exclude files from the zip build
	Excludes ConfigBuildZipPackExcludes `yaml:"excludes,omitempty"`
	// Commands to run before the pack
	BeforeHooks []string `yaml:"before_hooks,omitempty"`
}

type ConfigExtraBundle struct {
	// Path to the bundle, relative from the extension root (src folder)
	Path string `yaml:"path"`
	// Name of the bundle, if empty the folder name of path will be used
	Name string `yaml:"name"`
}

type ConfigStore struct {
	// Specifies the visibility in stores.
	Availabilities *[]string `yaml:"availabilities" jsonschema:"enum=German,enum=International"`
	// Specifies the default locale.
	DefaultLocale *string `yaml:"default_locale" jsonschema:"enum=de_DE,enum=en_GB"`
	// Specifies the languages the extension is translated.
	Localizations *[]string `yaml:"localizations" jsonschema:"enum=de_DE,enum=en_GB,enum=bs_BA,enum=bg_BG,enum=cs_CZ,enum=da_DK,enum=de_CH,enum=el_GR,enum=en_US,enum=es_ES,enum=fi_FI,enum=fr_FR,enum=hi_IN,enum=hr_HR,enum=hu_HU,enum=hy,enum=id_ID,enum=it_IT,enum=ko_KR,enum=lv_LV,enum=ms_MY,enum=nl_NL,enum=pl_PL,enum=pt_BR,enum=pt_PT,enum=ro_RO,enum=ru_RU,enum=sk_SK,enum=sl_SI,enum=sr_RS,enum=sv_SE,enum=th_TH,enum=tr_TR,enum=uk_UA,enum=vi_VN,enum=zh_CN,enum=zh_TW"`
	// Specifies the categories.
	Categories *[]string `yaml:"categories" jsonschema:"enum=Administration,enum=SEOOptimierung,enum=Bonitaetsprüfung,enum=Rechtssicherheit,enum=Auswertung,enum=KommentarFeedback,enum=Tracking,enum=Integration,enum=PreissuchmaschinenPortale,enum=Warenwirtschaft,enum=Versand,enum=Bezahlung,enum=StorefrontDetailanpassungen,enum=Sprache,enum=Suche,enum=HeaderFooter,enum=Detailseite,enum=MenueKategorien,enum=Bestellprozess,enum=KundenkontoPersonalisierung,enum=Sonderfunktionen,enum=Themes,enum=Branche,enum=Home+Furnishings,enum=FashionBekleidung,enum=GartenNatur,enum=KosmetikGesundheit,enum=EssenTrinken,enum=KinderPartyGeschenke,enum=SportLifestyleReisen,enum=Bauhaus,enum=Elektronik,enum=Geraete,enum=Heimkueche,enum=Hobby,enum=Kueche,enum=Lebensmittel,enum=Medizin,enum=Mode,enum=Musik,enum=Spiel,enum=Technik,enum=Umweltschutz,enum=Wohnen,enum=Zubehoer"`
	// Specifies the type of the extension.
	Type *string `yaml:"type" jsonschema:"enum=extension,enum=theme"`
	// Specifies the Path to the icon (128x128 px) for store.
	Icon *string `yaml:"icon"`
	// Specifies whether the extension should automatically be set compatible with Shopware bugfix versions.
	AutomaticBugfixVersionCompatibility *bool `yaml:"automatic_bugfix_version_compatibility"`
	// Specifies the meta title of the extension in store.
	MetaTitle ConfigTranslated[string] `yaml:"meta_title" jsonschema:"maxLength=50"`
	// Specifies the meta description of the extension in store.
	MetaDescription ConfigTranslated[string] `yaml:"meta_description" jsonschema:"maxLength=185"`
	// Specifies the description of the extension in store.
	Description ConfigTranslated[string] `yaml:"description"`
	// Installation manual of the extension in store.
	InstallationManual ConfigTranslated[string] `yaml:"installation_manual"`
	// Specifies the tags of the extension.
	Tags ConfigTranslated[[]string] `yaml:"tags,omitempty"`
	// Specifies the links of YouTube-Videos to show or describe the extension.
	Videos ConfigTranslated[[]string] `yaml:"videos,omitempty"`
	// Specifies the highlights of the extension.
	Highlights ConfigTranslated[[]string] `yaml:"highlights,omitempty"`
	// Specifies the features of the extension.
	Features ConfigTranslated[[]string] `yaml:"features"`
	// Specifies Frequently Asked Questions for the extension.
	Faq ConfigTranslated[[]ConfigStoreFaq] `yaml:"faq"`
	// Specifies images for the extension in the store.
	Images *[]ConfigStoreImage `yaml:"images,omitempty"`
	// Specifies the directory where the images are located.
	ImageDirectory *string `yaml:"image_directory,omitempty"`
}

type Translatable interface {
	string | []string | []ConfigStoreFaq
}

type ConfigTranslated[T Translatable] struct {
	German  *T `yaml:"de,omitempty"`
	English *T `yaml:"en,omitempty"`
}

type ConfigStoreFaq struct {
	Question string `yaml:"question"`
	Answer   string `yaml:"answer"`
}

type ConfigStoreImage struct {
	// File path to image relative from root of the extension
	File string `yaml:"file"`
	// Specifies whether the image is active in the language.
	Activate ConfigStoreImageActivate `yaml:"activate"`
	// Specifies whether the image is a preview in the language.
	Preview ConfigStoreImagePreview `yaml:"preview"`
	// Specifies the order of the image ascending the given priority.
	Priority int `yaml:"priority"`
}

type ConfigStoreImageActivate struct {
	German  bool `yaml:"de"`
	English bool `yaml:"en"`
}

type ConfigStoreImagePreview struct {
	German  bool `yaml:"de"`
	English bool `yaml:"en"`
}

// ConfigValidation is used to configure the extension validation.
type ConfigValidation struct {
	// Ignore items from the validation.
	Ignore ConfigValidationList `yaml:"ignore,omitempty"`
}

type ConfigValidationList []ConfigValidationIgnoreItem

func (c *ConfigValidationList) Identifiers() []string {
	identifiers := []string{}
	for _, item := range *c {
		identifiers = append(identifiers, item.Identifier)
	}
	return identifiers
}

type ConfigValidationIgnoreItem struct {
	// The identifier of the item to ignore.
	Identifier string `yaml:"identifier"`
	// Optional to ignore only a specific path.
	Path string `yaml:"path,omitempty"`
	// Optional to ignore only a specific message.
	Message string `yaml:"message,omitempty"`
}

func (c *ConfigValidationIgnoreItem) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		c.Identifier = value.Value
		return nil
	}

	type objectFormat struct {
		Identifier string `yaml:"identifier"`
		Path       string `yaml:"path,omitempty"`
		Message    string `yaml:"message,omitempty"`
	}
	var obj objectFormat
	if err := value.Decode(&obj); err != nil {
		return fmt.Errorf("failed to decode ConfigItem: %w", err)
	}

	c.Identifier = obj.Identifier
	c.Path = obj.Path
	c.Message = obj.Message

	return nil
}

func (c ConfigValidationIgnoreItem) JSONSchema() *jsonschema.Schema {
	ordMap := orderedmap.New[string, *jsonschema.Schema]()

	ordMap.Set("identifier", &jsonschema.Schema{
		Type:        "string",
		Description: "The identifier of the item to ignore.",
	})

	ordMap.Set("path", &jsonschema.Schema{
		Type:        "string",
		Description: "The path of the item to ignore.",
	})

	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{
				Type:       "object",
				Properties: ordMap,
			},
			{
				Type: "string",
			},
		},
	}
}

type Config struct {
	FileName string `yaml:"-" jsonschema:"-"`
	// Store is the store configuration of the extension.
	Store ConfigStore `yaml:"store,omitempty"`
	// Build is the build configuration of the extension.
	Build ConfigBuild `yaml:"build,omitempty"`
	// Changelog is the changelog configuration of the extension.
	Changelog changelog.Config `yaml:"changelog,omitempty"`
	// Validation is the validation configuration of the extension.
	Validation ConfigValidation `yaml:"validation,omitempty"`
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

func (c *Config) Dump(dir string) error {
	filePath := filepath.Join(dir, c.FileName)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("failed to close file: %v", err)
		}
	}()

	encoder := yaml.NewEncoder(file)
	defer func() {
		if err := encoder.Close(); err != nil {
			log.Printf("failed to close encoder: %v", err)
		}
	}()

	return encoder.Encode(c)
}
