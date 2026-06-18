package shop

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"dario.cat/mergo"
	"github.com/invopop/jsonschema"
	orderedmap "github.com/pb33f/ordered-map/v2"
	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/compatibility"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

type Config struct {
	AdditionalConfigs []string `yaml:"include,omitempty"`
	// The URL of the Shopware instance
	URL string `yaml:"url"`
	// Controls date-based compatibility behavior, formatted as YYYY-MM-DD.
	CompatibilityDate string            `yaml:"compatibility_date,omitempty" jsonschema:"format=date"`
	Build             *ConfigBuild      `yaml:"build,omitempty"`
	AdminApi          *ConfigAdminApi   `yaml:"admin_api,omitempty"`
	ConfigDump        *ConfigDump       `yaml:"dump,omitempty"`
	ConfigDeployment  *ConfigDeployment `yaml:"deployment,omitempty"`
	Validation        *ConfigValidation `yaml:"validation,omitempty"`
	ImageProxy        *ConfigImageProxy `yaml:"image_proxy,omitempty"`
	// When enabled, composer scripts will be disabled during CI builds
	DisableComposerScripts bool `yaml:"disable_composer_scripts,omitempty"`
	// When enabled, composer install will be skipped during CI builds
	DisableComposerInstall bool `yaml:"disable_composer_install,omitempty"`
	foundConfig            bool
}

func (c *Config) IsAdminAPIConfigured() bool {
	if c.AdminApi == nil {
		return false
	}

	return (c.AdminApi.ClientId != "" && c.AdminApi.ClientSecret != "") || (c.AdminApi.Username != "" && c.AdminApi.Password != "")
}

func (c *Config) HasCompatibilityDate() bool {
	return c.CompatibilityDate != ""
}

func (c *Config) IsCompatibilityDateAtLeast(requiredDate string) (bool, error) {
	return compatibility.IsAtLeast(c.CompatibilityDate, requiredDate)
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
	// When enabled, the shopware admin will be built
	ForceAdminBuild bool `yaml:"force_admin_build,omitempty"`
	// Keep following node_modules in the final build
	KeepNodeModules []string `yaml:"keep_node_modules,omitempty"`
	// MJML email template compilation configuration
	MJML *ConfigBuildMJML `yaml:"mjml,omitempty"`
	// When enabled, built assets are cached and restored on subsequent builds when sources haven't changed
	AssetCaching bool `yaml:"asset_caching,omitempty"`
	// Hooks to run at specific points during CI builds
	Hooks *ConfigBuildHooks `yaml:"hooks,omitempty"`
	// Shopware bundles to include in builds (alternative to composer.json extra.shopware-bundles)
	Bundles []ConfigProjectBundle `yaml:"bundles,omitempty"`
}

// ConfigProjectBundle defines a project-level Shopware bundle.
type ConfigProjectBundle struct {
	// Relative path from project root to the bundle directory
	Path string `yaml:"path" jsonschema:"required"`
	// Optional override for the bundle name; defaults to the directory basename
	Name string `yaml:"name,omitempty"`
}

// ConfigBuildHooks defines hooks to run at specific points during CI builds.
type ConfigBuildHooks struct {
	// Commands to run before anything runs
	Pre []string `yaml:"pre,omitempty"`
	// Commands to run after everything completes
	Post []string `yaml:"post,omitempty"`
	// Commands to run before composer install
	PreComposer []string `yaml:"pre-composer,omitempty"`
	// Commands to run after composer install
	PostComposer []string `yaml:"post-composer,omitempty"`
	// Commands to run before asset build
	PreAssets []string `yaml:"pre-assets,omitempty"`
	// Commands to run after asset build
	PostAssets []string `yaml:"post-assets,omitempty"`
}

func (c ConfigBuild) IsMjmlEnabled() bool {
	if c.MJML == nil {
		return false
	}

	return c.MJML.Enabled
}

// ConfigBuildExtension defines the configuration for forcing extension builds.
type ConfigBuildExtension struct {
	// Name of the extension
	Name string `yaml:"name" jsonschema:"required"`
}

// ConfigBuildMJML defines the configuration for MJML email template compilation.
type ConfigBuildMJML struct {
	// Whether to enable MJML compilation
	Enabled bool `yaml:"enabled,omitempty"`
	// Directories to search for MJML files
	SearchPaths []string `yaml:"search_paths,omitempty"`
	// When enabled, mj-include directives in MJML templates are processed.
	// MJML 5 ignores mj-include by default for security reasons; set this to
	// true to opt back in. Each search_path is automatically added to the
	// mj-include allowlist for files compiled inside it, so templates can
	// include siblings under the same search_path (e.g. a shared _includes/
	// folder) without further configuration.
	AllowIncludes bool `yaml:"allow_includes,omitempty"`
	// Extra directories outside any search_path that mj-include is allowed to
	// read from. Relative paths are resolved against the project root.
	// Absolute paths are used as-is. Most projects do not need this — set it
	// only when partials live outside the search_path tree. Implies
	// allow_includes.
	IncludePaths []string `yaml:"include_paths,omitempty"`
}

func (c ConfigBuildMJML) GetPaths(projectRoot string) []string {
	if len(c.SearchPaths) > 0 {
		absolutePaths := make([]string, len(c.SearchPaths))
		for i, path := range c.SearchPaths {
			if filepath.IsAbs(path) {
				absolutePaths[i] = path
			} else {
				absolutePaths[i] = filepath.Join(projectRoot, path)
			}
		}

		return absolutePaths
	}

	return []string{
		filepath.Join(projectRoot, "custom", "plugins"),
		filepath.Join(projectRoot, "custom", "static-plugins"),
	}
}

// ResolveIncludePaths returns IncludePaths as absolute paths. Relative entries
// are resolved against projectRoot; absolute entries are returned unchanged.
// Returns nil when no paths are configured.
func (c ConfigBuildMJML) ResolveIncludePaths(projectRoot string) []string {
	if len(c.IncludePaths) == 0 {
		return nil
	}

	resolved := make([]string, len(c.IncludePaths))
	for i, p := range c.IncludePaths {
		if filepath.IsAbs(p) {
			resolved[i] = p
		} else {
			resolved[i] = filepath.Join(projectRoot, p)
		}
	}
	return resolved
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
	Rewrite map[string]map[string]string `yaml:"rewrite,omitempty"`
	// Only export the schema of these tables, supports glob wildcards (e.g. "customer*")
	NoData []string `yaml:"nodata,omitempty"`
	// Ignore these tables from export, supports glob wildcards (e.g. "customer*")
	Ignore []string `yaml:"ignore,omitempty"`
	// Add an where condition to that table, schema is table name as key, and where statement as value
	Where map[string]string `yaml:"where,omitempty"`
}

// EnableClean adds default tables that should be excluded from data dump in clean mode
func (c *ConfigDump) EnableClean() {
	cleanTables := []string{
		"cart",
		"customer_recovery",
		"dead_message",
		"enqueue",
		"messenger_messages",
		"import_export_log",
		"increment",
		"elasticsearch_index_task",
		"log_entry",
		"message_queue_stats",
		"notification",
		"payment_token",
		"refresh_token",
		"version",
		"version_commit",
		"version_commit_data",
		"webhook_event_log",
	}
	for _, table := range cleanTables {
		if !slices.Contains(c.NoData, table) {
			c.NoData = append(c.NoData, table)
		}
	}
}

// EnableAnonymization adds default column rewrites for anonymizing customer data
func (c *ConfigDump) EnableAnonymization() {
	if c.Rewrite == nil {
		c.Rewrite = make(map[string]map[string]string)
	}

	anonymizationRewrites := map[string]map[string]string{
		"customer": {
			"first_name":     "faker.Person.FirstName()",
			"last_name":      "faker.Person.LastName()",
			"company":        "faker.Person.Name()",
			"title":          "faker.Person.Name()",
			"email":          "faker.Internet.Email()",
			"remote_address": "faker.Internet.Ipv4()",
		},
		"customer_address": {
			"first_name":   "faker.Person.FirstName()",
			"last_name":    "faker.Person.LastName()",
			"company":      "faker.Person.Name()",
			"title":        "faker.Person.Name()",
			"street":       "faker.Address.StreetAddress()",
			"zipcode":      "faker.Address.PostCode()",
			"city":         "faker.Address.City()",
			"phone_number": "faker.Phone.Number()",
		},
		"log_entry": {
			"provider": "",
		},
		"newsletter_recipient": {
			"email":      "faker.Internet.Email()",
			"first_name": "faker.Person.FirstName()",
			"last_name":  "faker.Person.LastName()",
			"city":       "faker.Address.City()",
		},
		"order_address": {
			"first_name":   "faker.Person.FirstName()",
			"last_name":    "faker.Person.LastName()",
			"company":      "faker.Person.Name()",
			"title":        "faker.Person.Name()",
			"street":       "faker.Address.StreetAddress()",
			"zipcode":      "faker.Address.PostCode()",
			"city":         "faker.Address.City()",
			"phone_number": "faker.Phone.Number()",
		},
		"order_customer": {
			"first_name":     "faker.Person.FirstName()",
			"last_name":      "faker.Person.LastName()",
			"company":        "faker.Person.Name()",
			"title":          "faker.Person.Name()",
			"email":          "faker.Internet.Email()",
			"remote_address": "faker.Internet.Ipv4()",
		},
		"product_review": {
			"email": "faker.Internet.Email()",
		},
	}

	// Merge with existing rewrites; user-supplied values take precedence over defaults
	for table, columns := range anonymizationRewrites {
		if _, exists := c.Rewrite[table]; !exists {
			c.Rewrite[table] = columns
			continue
		}

		for column, rewrite := range columns {
			if _, columnExists := c.Rewrite[table][column]; !columnExists {
				c.Rewrite[table][column] = rewrite
			}
		}
	}
}

// NormalizeFakerExpressions wraps bare faker expressions with {{- -}} delimiters
// so they can be properly evaluated by the mysqldump faker processor.
func (c *ConfigDump) NormalizeFakerExpressions() {
	if c.Rewrite == nil {
		return
	}

	for table, columns := range c.Rewrite {
		for column, value := range columns {
			trimmed := strings.TrimSpace(value)
			if strings.HasPrefix(trimmed, "faker.") && !strings.Contains(value, "{{-") {
				c.Rewrite[table][column] = "{{- " + trimmed + " -}}"
			}
		}
	}
}

type ConfigDeployment struct {
	Hooks struct {
		// The pre hook will be executed before the deployment
		Pre ConfigDeploymentHook `yaml:"pre"`
		// The post hook will be executed after the deployment
		Post ConfigDeploymentHook `yaml:"post"`
		// The pre-install hook will be executed before the installation
		PreInstall ConfigDeploymentHook `yaml:"pre-install"`
		// The post-install hook will be executed after the installation
		PostInstall ConfigDeploymentHook `yaml:"post-install"`
		// The pre-update hook will be executed before the update
		PreUpdate ConfigDeploymentHook `yaml:"pre-update"`
		// The post-update hook will be executed after the update
		PostUpdate ConfigDeploymentHook `yaml:"post-update"`
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

		// DEPRECATED, On these extensions, it will be always called plugin:update
		ForceUpdatesDeprecated []string `yaml:"force_updates,omitempty" jsonschema:"deprecated=true"`
		// On these extensions, it will be always called plugin:update
		ForceUpdate []string `yaml:"force-update,omitempty"`
	} `yaml:"extension-management"`

	OneTimeTasks []struct {
		Id     string `yaml:"id" jsonschema:"required"`
		Script string `yaml:"script" jsonschema:"required"`
	} `yaml:"one-time-tasks"`

	// Staging mode configuration for the deployment
	Staging *ConfigDeploymentStaging `yaml:"staging,omitempty"`
}

// ConfigDeploymentStaging defines staging mode configuration.
type ConfigDeploymentStaging struct {
	// When enabled, staging setup commands will be executed during installation and upgrade
	Enabled bool `yaml:"enabled,omitempty"`
}

// ConfigDeploymentHookStep is a single titled step of a deployment hook.
type ConfigDeploymentHookStep struct {
	// An optional title shown in the deployment output for this step
	Title string `yaml:"title,omitempty"`
	// The script that is executed for this step
	Script string `yaml:"script"`
}

// ConfigDeploymentHook is a deployment hook. It can either be a single script
// (string) or a list of steps that are executed individually. Each step can be
// a plain script string or an object with a "title" and a "script".
type ConfigDeploymentHook struct {
	Steps []ConfigDeploymentHookStep
}

func (h *ConfigDeploymentHook) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var script string
		if err := value.Decode(&script); err != nil {
			return err
		}

		h.Steps = nil
		if script != "" {
			h.Steps = []ConfigDeploymentHookStep{{Script: script}}
		}

		return nil
	case yaml.SequenceNode:
		steps := make([]ConfigDeploymentHookStep, 0, len(value.Content))
		for _, node := range value.Content {
			if node.Kind == yaml.ScalarNode {
				var script string
				if err := node.Decode(&script); err != nil {
					return err
				}

				steps = append(steps, ConfigDeploymentHookStep{Script: script})

				continue
			}

			var step ConfigDeploymentHookStep
			if err := node.Decode(&step); err != nil {
				return err
			}

			steps = append(steps, step)
		}

		h.Steps = steps

		return nil
	default:
		return fmt.Errorf("invalid hook: expected a script string or a list of steps")
	}
}

func (ConfigDeploymentHook) JSONSchema() *jsonschema.Schema {
	stepProperties := orderedmap.New[string, *jsonschema.Schema]()
	stepProperties.Set("title", &jsonschema.Schema{
		Type:        "string",
		Description: "An optional title shown in the deployment output for this step",
	})
	stepProperties.Set("script", &jsonschema.Schema{
		Type:        "string",
		Description: "The script that is executed for this step",
	})

	step := &jsonschema.Schema{
		Type:                 "object",
		Properties:           stepProperties,
		Required:             []string{"script"},
		AdditionalProperties: jsonschema.FalseSchema,
	}

	return &jsonschema.Schema{
		Description: "Either a single script or a list of steps (a script string or a {title, script} object) executed individually",
		OneOf: []*jsonschema.Schema{
			{Type: "string"},
			{
				Type: "array",
				Items: &jsonschema.Schema{
					OneOf: []*jsonschema.Schema{
						{Type: "string"},
						step,
					},
				},
			},
		},
	}
}

type ConfigDeploymentOverrides map[string]struct {
	State string `yaml:"state"`
}

func (c ConfigDeploymentOverrides) JSONSchema() *jsonschema.Schema {
	properties := orderedmap.New[string, *jsonschema.Schema]()

	properties.Set("state", &jsonschema.Schema{
		Type: "string",
		Enum: []interface{}{"inactive", "remove", "ignore", "installed"},
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

// ConfigValidation is used to configure the project validation.
type ConfigValidation struct {
	// Ignore items from the validation.
	Ignore []ConfigValidationIgnoreItem `yaml:"ignore,omitempty"`

	IgnoreExtensions []ConfigValidationIgnoreExtension `yaml:"ignore_extensions,omitempty"`

	// PhpVersion overrides the PHP version used for linting (e.g. "8.4").
	// When set, this takes precedence over the version derived from composer.json or the static Shopware-to-PHP mapping.
	PhpVersion string `yaml:"php_version,omitempty"`
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

type ConfigImageProxy struct {
	// The URL of the upstream server to proxy requests to when files are not found locally
	URL string `yaml:"url,omitempty"`
}

func ReadConfig(ctx context.Context, fileName string, allowFallback bool) (*Config, error) {
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

	localFile := localConfigFileName(fileName)
	_, localErr := os.Stat(localFile)
	if localErr != nil && !os.IsNotExist(localErr) {
		logging.FromContext(ctx).Warnf("unable to access local config override %s: %v", localFile, localErr)
	}
	hasLocalFile := localErr == nil

	if hasLocalFile {
		baseMap, err := readConfigAsMap(fileName)
		if err != nil {
			return nil, fmt.Errorf("ReadConfig(%s): %v", fileName, err)
		}

		mergedMap, err := mergeLocalConfig(baseMap, localFile)
		if err != nil {
			return nil, fmt.Errorf("ReadConfig(%s): %v", fileName, err)
		}

		mergedYAML, err := marshalMap(mergedMap)
		if err != nil {
			return nil, fmt.Errorf("ReadConfig(%s): %v", fileName, err)
		}

		if err := yaml.Unmarshal(mergedYAML, &config); err != nil {
			return nil, fmt.Errorf("ReadConfig(%s): %v", fileName, err)
		}
	} else {
		fileHandle, err := os.ReadFile(fileName)
		if err != nil {
			return nil, fmt.Errorf("ReadConfig(%s): %v", fileName, err)
		}

		substitutedConfig := system.ExpandEnv(string(fileHandle))
		if err := yaml.Unmarshal([]byte(substitutedConfig), &config); err != nil {
			return nil, fmt.Errorf("ReadConfig(%s): %v", fileName, err)
		}
	}

	config.foundConfig = true

	if len(config.AdditionalConfigs) > 0 {
		for _, additionalConfigFile := range config.AdditionalConfigs {
			additionalConfig, err := ReadConfig(ctx, additionalConfigFile, allowFallback)
			if err != nil {
				return nil, fmt.Errorf("error while reading included config: %s", err.Error())
			}

			err = mergo.Merge(config, additionalConfig, mergo.WithOverride, mergo.WithSliceDeepCopy)
			if err != nil {
				return nil, fmt.Errorf("error while merging included config: %s", err.Error())
			}
		}
	}

	if config.foundConfig && config.CompatibilityDate == "" {
		logging.FromContext(ctx).Warnf("Config %s is missing compatibility_date, defaulting to %s", fileName, compatibility.DefaultDate())
	}

	if err := compatibility.ValidateDate(config.CompatibilityDate); err != nil {
		return nil, fmt.Errorf("ReadConfig(%s): %v", fileName, err)
	}

	return fillEmptyConfig(config), nil
}

func fillEmptyConfig(c *Config) *Config {
	if c.CompatibilityDate == "" {
		c.CompatibilityDate = compatibility.DefaultDate()
	}

	if c.Build == nil {
		c.Build = &ConfigBuild{}
	}

	return c
}

func (c Config) IsFallback() bool {
	return !c.foundConfig
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
