package shop

import (
	"fmt"
	"os"
	"path"
	"strings"

	"dario.cat/mergo"
	"github.com/doutorfinancas/go-mad/core"
	adminSdk "github.com/friendsofshopware/go-shopware-admin-api-sdk"
	"github.com/google/uuid"
	"github.com/invopop/jsonschema"
	orderedmap "github.com/wk8/go-ordered-map/v2"
	"gopkg.in/yaml.v3"
)

type Config struct {
	AdditionalConfigs []string `yaml:"include,omitempty"`
	// The URL of the Shopware instance
	URL              string            `yaml:"url"`
	Build            *ConfigBuild      `yaml:"build,omitempty"`
	AdminApi         *ConfigAdminApi   `yaml:"admin_api,omitempty"`
	ConfigDump       *ConfigDump       `yaml:"dump,omitempty"`
	Sync             *ConfigSync       `yaml:"sync,omitempty"`
	ConfigDeployment *ConfigDeployment `yaml:"deployment,omitempty"`
	Validation       *ConfigValidation `yaml:"validation,omitempty"`
	foundConfig      bool
}

type ConfigBuild struct {
	// When enabled, the assets will not be copied to the public folder
	DisableAssetCopy bool `yaml:"disable_asset_copy,omitempty"`
	// When enabled, the assets of extensions will be removed from the extension public folder. (Requires Shopware 6.5.2.0)
	RemoveExtensionAssets bool `yaml:"remove_extension_assets,omitempty"`
	// When enabled, the extensions source code will be keep in the final build
	KeepExtensionSource bool `yaml:"keep_extension_source,omitempty"`
	// When enabled, the source maps will not be removed from the final build
	KeepSourceMaps bool `yaml:"keep_source_maps,omitempty"`
	// Paths to delete for the final build
	CleanupPaths []string `yaml:"cleanup_paths,omitempty"`
	// Browserslist configuration for the Storefront build
	Browserslist string `yaml:"browserslist,omitempty"`
	// Extensions to exclude from the build
	ExcludeExtensions []string `yaml:"exclude_extensions,omitempty"`
	// When enabled, the storefront build will be skipped
	DisableStorefrontBuild bool `yaml:"disable_storefront_build,omitempty"`
	// Extensions to force build for, even if they have compiled files
	ForceExtensionBuild []ConfigBuildExtension `yaml:"force_extension_build,omitempty"`
}

// ConfigBuildExtension defines the configuration for forcing extension builds.
type ConfigBuildExtension struct {
	// Name of the extension
	Name string `yaml:"name" jsonschema:"required"`
}

type ConfigAdminApi struct {
	// Client ID of integration
	ClientId string `yaml:"client_id,omitempty"`
	// Client Secret of integration
	ClientSecret string `yaml:"client_secret,omitempty"`
	// Username of admin user
	Username string `yaml:"username,omitempty"`
	// Password of admin user
	Password string `yaml:"password,omitempty"`
	// Disable SSL certificate check
	DisableSSLCheck bool `yaml:"disable_ssl_check,omitempty"`
}

type ConfigDump struct {
	// Allows to rewrite single columns, perfect for GDPR compliance
	Rewrite map[string]core.Rewrite `yaml:"rewrite,omitempty"`
	// Only export the schema of these tables
	NoData []string `yaml:"nodata,omitempty"`
	// Ignore these tables from export
	Ignore []string `yaml:"ignore,omitempty"`
	// Add an where condition to that table, schema is table name as key, and where statement as value
	Where map[string]string `yaml:"where,omitempty"`
}

type ConfigSync struct {
	Enabled      *[]string          `yaml:"enabled,omitempty" jsonschema:"enum=system_config,enum=mail_template,enum=theme,enum=entity"`
	Config       []ConfigSyncConfig `yaml:"config,omitempty"`
	Theme        []ThemeConfig      `yaml:"theme,omitempty"`
	MailTemplate []MailTemplate     `yaml:"mail_template,omitempty"`
	Entity       []EntitySync       `yaml:"entity,omitempty"`
}

type ConfigDeployment struct {
	Hooks struct {
		// The pre hook will be executed before the deployment
		Pre string `yaml:"pre"`
		// The post hook will be executed after the deployment
		Post string `yaml:"post"`
		// The pre-install hook will be executed before the installation
		PreInstall string `yaml:"pre-install"`
		// The post-install hook will be executed after the installation
		PostInstall string `yaml:"post-install"`
		// The pre-update hook will be executed before the update
		PreUpdate string `yaml:"pre-update"`
		// The post-update hook will be executed after the update
		PostUpdate string `yaml:"post-update"`
	} `yaml:"hooks"`

	Store struct {
		LicenseDomain string `yaml:"license-domain"`
	} `yaml:"store"`

	Cache struct {
		AlwaysClear bool `yaml:"always_clear"`
	} `yaml:"cache"`

	// The extension management of the deployment
	ExtensionManagement struct {
		// When enabled, the extensions will be installed, updated, and removed
		Enabled bool `yaml:"enabled"`
		// Which extensions should not be managed
		Exclude []string `yaml:"exclude"`

		Overrides ConfigDeploymentOverrides `yaml:"overrides"`
	} `yaml:"extension-management"`

	OneTimeTasks []struct {
		Id     string `yaml:"id" jsonschema:"required"`
		Script string `yaml:"script" jsonschema:"required"`
	} `yaml:"one-time-tasks"`
}

type ConfigDeploymentOverrides map[string]struct {
	State string `yaml:"state"`
}

func (c ConfigDeploymentOverrides) JSONSchema() *jsonschema.Schema {
	properties := orderedmap.New[string, *jsonschema.Schema]()

	properties.Set("state", &jsonschema.Schema{
		Type: "string",
		Enum: []interface{}{"inactive", "remove", "ignore"},
	})

	properties.Set("keepUserData", &jsonschema.Schema{
		Type: "boolean",
	})

	return &jsonschema.Schema{
		Type:  "object",
		Title: "Extension overrides",
		AdditionalProperties: &jsonschema.Schema{
			Type:       "object",
			Properties: properties,
			Required:   []string{"state"},
		},
	}
}

type ConfigSyncConfig struct {
	// Sales Channel ID to apply
	SalesChannel *string `yaml:"sales_channel,omitempty"`
	// Configurations of that Sales Channel
	Settings map[string]interface{} `yaml:"settings"`
}

type ThemeConfig struct {
	Name     string                               `yaml:"name"`
	Settings map[string]adminSdk.ThemeConfigValue `yaml:"settings"`
}

type MailTemplate struct {
	Id           string                    `yaml:"id"`
	Translations []MailTemplateTranslation `yaml:"translations"`
}

type EntitySync struct {
	Entity  string                 `yaml:"entity"`
	Exists  *[]EntitySyncFilter    `yaml:"exists,omitempty"`
	Payload map[string]interface{} `yaml:"payload"`
}

type EntitySyncFilter struct {
	// The type of filter
	Type string `yaml:"type" jsonschema:"required,enum=equals,enum=multi,enum=contains,enum=prefix,enum=suffix,enum=not,enum=range,enum=until,enum=equalsAll,enum=equalsAny"`
	// The field to filter on
	Field string `yaml:"field" jsonschema:"required"`
	// The actual filter value
	Value interface{} `yaml:"value"`
	// The operator to use for multiple filters
	Operator *string `yaml:"operator,omitempty" jsonschema:"enum=AND,enum=OR,enum=XOR"`
	// The filters to apply, when type set to multi
	Queries *[]EntitySyncFilter `yaml:"queries,omitempty"`
}

func (s EntitySyncFilter) JSONSchema() *jsonschema.Schema {
	properties := orderedmap.New[string, *jsonschema.Schema]()

	properties.Set("type", &jsonschema.Schema{
		Type: "string",
		Enum: []interface{}{"equals", "multi", "contains", "prefix", "suffix", "not", "range", "until", "equalsAll", "equalsAny"},
	})

	properties.Set("field", &jsonschema.Schema{
		Type:        "string",
		Description: "The field to filter on",
	})

	properties.Set("value", &jsonschema.Schema{
		Description: "The actual filter value",
	})

	properties.Set("operator", &jsonschema.Schema{
		Type: "string",
		Enum: []interface{}{"AND", "OR", "XOR"},
	})

	ifProperties := orderedmap.New[string, *jsonschema.Schema]()
	ifProperties.Set("type", &jsonschema.Schema{
		Const: "multi",
	})

	return &jsonschema.Schema{
		Type:       "object",
		Title:      "Entity Sync Filter",
		Properties: properties,
		Required:   []string{"type", "field"},
		AllOf: []*jsonschema.Schema{
			{
				If: &jsonschema.Schema{
					Properties: ifProperties,
					Then: &jsonschema.Schema{
						Required: []string{"type", "queries"},
					},
				},
			},
		},
	}
}

type MailTemplateTranslation struct {
	Language     string      `yaml:"language"`
	SenderName   string      `yaml:"sender_name"`
	Subject      string      `yaml:"subject"`
	HTML         string      `yaml:"html"`
	Plain        string      `yaml:"plain"`
	CustomFields interface{} `yaml:"custom_fields"`
}

// ConfigValidation is used to configure the project validation.
type ConfigValidation struct {
	// Ignore items from the validation.
	Ignore []ConfigValidationIgnoreItem `yaml:"ignore,omitempty"`

	IgnoreExtensions []ConfigValidationIgnoreExtension `yaml:"ignore_extensions,omitempty"`
}

// ConfigValidationIgnoreItem is used to ignore items from the validation.
type ConfigValidationIgnoreItem struct {
	// The identifier of the item to ignore.
	Identifier string `yaml:"identifier"`
	// The path of the item to ignore.
	Path string `yaml:"path,omitempty"`
	// The message of the item to ignore.
	Message string `yaml:"message,omitempty"`
}

type ConfigValidationIgnoreExtension struct {
	// The name of the extension to ignore.
	Name string `yaml:"name"`
}

func ReadConfig(fileName string, allowFallback bool) (*Config, error) {
	config := &Config{foundConfig: false}

	_, err := os.Stat(fileName)

	if os.IsNotExist(err) {
		if allowFallback {
			return fillEmptyConfig(config), nil
		}

		return nil, fmt.Errorf("cannot find project configuration file \"%s\", use shopware-cli project config init to create one", fileName)
	}

	if err != nil {
		return nil, err
	}

	fileHandle, err := os.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("ReadConfig (%s): %v", fileName, err)
	}

	config.foundConfig = true

	substitutedConfig := os.ExpandEnv(string(fileHandle))
	err = yaml.Unmarshal([]byte(substitutedConfig), &config)

	if len(config.AdditionalConfigs) > 0 {
		for _, additionalConfigFile := range config.AdditionalConfigs {
			additionalConfig, err := ReadConfig(additionalConfigFile, allowFallback)
			if err != nil {
				return nil, fmt.Errorf("error while reading included config: %s", err.Error())
			}

			err = mergo.Merge(additionalConfig, config, mergo.WithOverride, mergo.WithSliceDeepCopy)
			if err != nil {
				return nil, fmt.Errorf("error while merging included config: %s", err.Error())
			}

			config = additionalConfig
		}
	}

	if err != nil {
		return nil, fmt.Errorf("ReadConfig(%s): %v", fileName, err)
	}

	return fillEmptyConfig(config), nil
}

const (
	SyncOptionEntity       = "entity"
	SyncOptionMailTemplate = "mail_template"
	SyncOptionSystemConfig = "system_config"
	SyncOptionTheme        = "theme"
)

func fillEmptyConfig(c *Config) *Config {
	if c.Build == nil {
		c.Build = &ConfigBuild{}
	}

	if c.Sync == nil {
		c.Sync = &ConfigSync{}
	}

	return c
}

func (c Config) IsFallback() bool {
	return !c.foundConfig
}

func NewUuid() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")
}

func DefaultConfigFileName() string {
	currentDir, err := os.Getwd()
	if err != nil {
		return ".shopware-project.yml"
	}

	if _, err := os.Stat(path.Join(currentDir, ".shopware-project.yaml")); err == nil {
		return ".shopware-project.yaml"
	}

	return ".shopware-project.yml"
}
