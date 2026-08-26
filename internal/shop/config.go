package shop

import (
	"context"
	"errors"
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
	"github.com/shopware/shopware-cli/internal/mysqldump"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

type EnvironmentConfig struct {
	Type string `yaml:"type" jsonschema:"enum=local,enum=docker"`
	// Shop URL for this named environment
	URL string `yaml:"url,omitempty"`
	// Admin API credentials for this named environment
	AdminApi *ConfigAdminApi `yaml:"admin_api,omitempty"`
}

type Config struct {
	AdditionalConfigs []string `yaml:"include,omitempty"`
	// Shop URL. Prefer environments.local.url or another named environment; the top-level url key is still read during the deprecation window.
	URL string `yaml:"url,omitempty" jsonschema:"deprecated=true"`
	// Controls date-based compatibility behavior, formatted as YYYY-MM-DD.
	CompatibilityDate string `yaml:"compatibility_date,omitempty" jsonschema:"format=date"`
	// PHP version (e.g. "8.3") used for local PHP and Composer commands of this project. Written by "project create" for non-Docker projects. The matching PHP is looked up on the machine running the command, so the value stays portable across machines; it takes precedence over the php found in PATH, while the PHP_BINARY environment variable overrides it.
	PHPVersion string       `yaml:"php_version,omitempty"`
	Build      *ConfigBuild `yaml:"build,omitempty"`
	// Admin API credentials. Prefer environments.local.admin_api or another named environment; the top-level admin_api key is still read during the deprecation window.
	AdminApi         *ConfigAdminApi   `yaml:"admin_api,omitempty" jsonschema:"deprecated=true"`
	ConfigDump       *ConfigDump       `yaml:"dump,omitempty"`
	ConfigDeployment *ConfigDeployment `yaml:"deployment,omitempty"`
	Validation       *ConfigValidation `yaml:"validation,omitempty"`
	ImageProxy       *ConfigImageProxy `yaml:"image_proxy,omitempty"`
	// Docker dev environment configuration
	Docker *ConfigDocker `yaml:"docker,omitempty"`
	// Named shop targets. Empty -e selects environments.local. Store url and admin_api here rather than at the top level.
	Environments map[string]*EnvironmentConfig `yaml:"environments,omitempty"`
	// When enabled, composer scripts will be disabled during CI builds
	DisableComposerScripts bool `yaml:"disable_composer_scripts,omitempty"`
	// When enabled, composer install will be skipped during CI builds
	DisableComposerInstall bool `yaml:"disable_composer_install,omitempty"`
	foundConfig            bool
}

// ResolveEnvironment returns the named environment, or for an empty name
// environments.local with the deprecated top-level url/admin_api as fallback.
func (c *Config) ResolveEnvironment(name string) (*EnvironmentConfig, error) {
	if name != "" {
		env, ok := c.Environments[name]
		if !ok {
			return nil, fmt.Errorf("environment %q not found in config", name)
		}
		if env == nil {
			return nil, fmt.Errorf("environment %q has no configuration", name)
		}
		return env, nil
	}

	local, ok := c.Environments["local"]
	if !ok || local == nil {
		return c.topLevelEnvironment(nil), nil
	}

	if !c.HasDeprecatedTopLevelShop() {
		return local, nil
	}

	return c.topLevelEnvironment(local), nil
}

// topLevelEnvironment builds the environment from the deprecated top-level
// url/admin_api, using values set on base where present.
func (c *Config) topLevelEnvironment(base *EnvironmentConfig) *EnvironmentConfig {
	env := EnvironmentConfig{Type: "local"}
	if base != nil {
		env = *base
		if env.Type == "" {
			env.Type = "local"
		}
	}

	if env.URL == "" {
		env.URL = c.URL
	}

	if env.AdminApi == nil {
		env.AdminApi = c.AdminApi
	}

	return &env
}

// EffectiveURL returns the URL of the default environment:
// environments.local.url, falling back to the deprecated top-level url.
func (c *Config) EffectiveURL() string {
	if c == nil {
		return ""
	}

	if env, ok := c.Environments["local"]; ok && env != nil && env.URL != "" {
		return env.URL
	}

	return c.URL
}

// HasDeprecatedTopLevelShop reports whether the config still stores a shop
// target at the top-level url or admin_api keys.
func (c *Config) HasDeprecatedTopLevelShop() bool {
	return c.URL != "" || c.AdminApi != nil
}

// SetLocalShop stores the shop URL and optional Admin API credentials on
// environments.local. It does not set top-level url or admin_api.
func (c *Config) SetLocalShop(url string, adminApi *ConfigAdminApi) {
	env := c.ensureLocalEnvironment()
	env.URL = url
	if adminApi != nil {
		env.AdminApi = adminApi
	}
}

func (c *Config) ensureLocalEnvironment() *EnvironmentConfig {
	if c.Environments == nil {
		c.Environments = make(map[string]*EnvironmentConfig)
	}
	if env := c.Environments["local"]; env != nil {
		return env
	}
	env := &EnvironmentConfig{Type: "local"}
	c.Environments["local"] = env
	return env
}

// WithEnvironment returns a copy of the config with URL and Admin API credentials from ResolveEnvironment, keeping base values the environment does not override.
func (c *Config) WithEnvironment(name string) (*Config, error) {
	env, err := c.ResolveEnvironment(name)
	if err != nil {
		return nil, err
	}

	cfg := *c

	if env.URL != "" {
		cfg.URL = env.URL
	}

	if env.AdminApi != nil {
		cfg.AdminApi = env.AdminApi
	}

	return &cfg, nil
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

func (c *Config) IsCompatibilityDateBefore(requiredDate string) bool {
	return compatibility.IsBefore(c.CompatibilityDate, requiredDate)
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
	// When enabled, the checksum.json generation for extensions will be skipped
	DisableChecksums bool `yaml:"disable_checksums,omitempty"`
	// When enabled, an already existing checksum.json in an extension will be kept instead of being overwritten
	KeepExistingChecksums bool `yaml:"keep_existing_checksums,omitempty"`
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
	// Limit the amount of exported rows of a table, schema is table name as key. All tables referencing the limited table via foreign keys (also transitively) are filtered automatically, so they only contain rows belonging to the kept rows. A second limit on a table that is already filtered this way is rejected. When the limited table references itself (e.g. product.parent_id), the ancestors of the kept rows are exported too, so the dump stays importable. Requires the CREATE and DROP privileges to freeze the kept rows into staging tables
	Limit map[string]mysqldump.TableLimit `yaml:"limit,omitempty"`
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
	if value.Kind == yaml.ScalarNode {
		var script string
		if err := value.Decode(&script); err != nil {
			return err
		}

		h.Steps = nil
		if script != "" {
			h.Steps = []ConfigDeploymentHookStep{{Script: script}}
		}

		return nil
	}

	if value.Kind == yaml.SequenceNode {
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
	}

	return errors.New("invalid hook: expected a script string or a list of steps")
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

type ConfigDocker struct {
	// PHP configuration for the Docker dev image
	PHP *ConfigDockerPHP `yaml:"php,omitempty"`
}

type ConfigDockerPHP struct {
	// PHP version (e.g. "8.3", "8.2"). Defaults to "8.3".
	Version string `yaml:"version,omitempty"`
	// Profiler to enable. Possible values: xdebug, blackfire, tideways, pcov, spx.
	Profiler string `yaml:"profiler,omitempty" jsonschema:"enum=xdebug,enum=blackfire,enum=tideways,enum=pcov,enum=spx"`
	// Blackfire server ID from your Blackfire account. Required when profiler is "blackfire".
	BlackfireServerID string `yaml:"blackfire_server_id,omitempty"`
	// Blackfire server token from your Blackfire account. Required when profiler is "blackfire".
	BlackfireServerToken string `yaml:"blackfire_server_token,omitempty"`
	// Tideways API key from your Tideways account. Required when profiler is "tideways".
	TidewaysAPIKey string `yaml:"tideways_api_key,omitempty"`
}

func (ConfigDockerPHP) JSONSchema() *jsonschema.Schema {
	properties := orderedmap.New[string, *jsonschema.Schema]()

	properties.Set("version", &jsonschema.Schema{
		Type:        "string",
		Description: "PHP version (e.g. \"8.3\", \"8.2\"). Defaults to \"8.3\".",
	})

	properties.Set("profiler", &jsonschema.Schema{
		Type:        "string",
		Enum:        []any{"xdebug", "blackfire", "tideways", "pcov", "spx"},
		Description: "Profiler to enable. Possible values: xdebug, blackfire, tideways, pcov, spx.",
	})

	properties.Set("blackfire_server_id", &jsonschema.Schema{
		Type:        "string",
		Description: "Blackfire server ID from your Blackfire account. Required when profiler is \"blackfire\".",
	})

	properties.Set("blackfire_server_token", &jsonschema.Schema{
		Type:        "string",
		Description: "Blackfire server token from your Blackfire account. Required when profiler is \"blackfire\".",
	})

	properties.Set("tideways_api_key", &jsonschema.Schema{
		Type:        "string",
		Description: "Tideways API key from your Tideways account. Required when profiler is \"tideways\".",
	})

	profilerConst := func(value string) *orderedmap.OrderedMap[string, *jsonschema.Schema] {
		m := orderedmap.New[string, *jsonschema.Schema]()
		m.Set("profiler", &jsonschema.Schema{Const: value})
		return m
	}

	return &jsonschema.Schema{
		Type:                 "object",
		Properties:           properties,
		AdditionalProperties: jsonschema.FalseSchema,
		AllOf: []*jsonschema.Schema{
			{
				If: &jsonschema.Schema{
					Properties: profilerConst("blackfire"),
					Required:   []string{"profiler"},
				},
				Then: &jsonschema.Schema{
					Required: []string{"blackfire_server_id", "blackfire_server_token"},
				},
			},
			{
				If: &jsonschema.Schema{
					Properties: profilerConst("tideways"),
					Required:   []string{"profiler"},
				},
				Then: &jsonschema.Schema{
					Required: []string{"tideways_api_key"},
				},
			},
		},
	}
}

type ConfigImageProxy struct {
	// The URL of the upstream server to proxy requests to when files are not found locally
	URL string `yaml:"url,omitempty"`
}

func NewConfig() *Config {
	return &Config{
		CompatibilityDate: compatibility.TodayDate(),
		Environments: map[string]*EnvironmentConfig{
			"local": {
				Type: "local",
				URL:  "http://127.0.0.1:8000",
				AdminApi: &ConfigAdminApi{
					Username: "admin",
					Password: "shopware",
				},
			},
		},
	}
}

func WriteConfig(cfg *Config, dir string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal shop configuration: %w", err)
	}

	filePath := filepath.Join(dir, ".shopware-project.yml")

	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write shop configuration to %s: %w", filePath, err)
	}

	return nil
}

// WriteLocalConfig writes a partial configuration to .shopware-project.local.yml.
// This file is deep-merged on top of the main config at read time and is intended
// for credentials and other values that should not be committed to version control.
func WriteLocalConfig(cfg *Config, dir string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal local shop configuration: %w", err)
	}

	filePath := filepath.Join(dir, ".shopware-project.local.yml")

	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write local shop configuration to %s: %w", filePath, err)
	}

	return nil
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
	warnDeprecatedTopLevelShop(ctx, fileName, config)

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

func warnDeprecatedTopLevelShop(ctx context.Context, fileName string, c *Config) {
	if !c.HasDeprecatedTopLevelShop() {
		return
	}

	logging.FromContext(ctx).Warnf(
		"Config %s uses deprecated top-level url/admin_api; move shop URL and Admin API credentials to environments (empty -e defaults to environments.local)",
		fileName,
	)
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

// --- In-place url patching -------------------------------------------------
//
// `project proxy up` needs to repoint a project at its proxy hostname by
// changing just the `url:` key, and `proxy down` needs to put the old value
// back exactly. ReadConfig/WriteConfig cannot do this: WriteConfig re-marshals
// the whole Config struct, so it would reorder every key and drop the user's
// comments (a huge diff in a committed file for a one-line change), and
// ReadConfig merges the .local override and cannot tell an absent `url:` from
// an empty one. The helpers below therefore edit the YAML document in place
// (via yaml.Node), touching only the url keys and leaving everything else —
// comments, ordering, unknown keys — untouched.

// ConfigURLState captures the url values of a project config file
// (.shopware-project.yml) before proxy registration, so deregistration can
// restore them exactly. The rest of the CLI (dev TUI, admin API client)
// resolves the shop URL from these keys, which is why registration points
// them at the proxy hostname.
type ConfigURLState struct {
	// HasFile is false when the project has no config file; registration
	// then leaves the project config alone entirely.
	HasFile bool `json:"-"`
	// RootURL is the top-level url value. HasRoot false = key was absent.
	RootURL string `json:"root_url,omitempty"`
	HasRoot bool   `json:"had_root_url,omitempty"`
	// EnvURL is environments.<env>.url, which overrides the top-level url
	// in the dev TUI and the admin API client. HasEnv false = key absent.
	EnvURL string `json:"env_url,omitempty"`
	HasEnv bool   `json:"had_env_url,omitempty"`
}

// urlEnvKey resolves the environments map key the CLI would use: an explicit
// environment name, or "local" (mirroring Config.ResolveEnvironment).
func urlEnvKey(envName string) string {
	if envName == "" {
		return "local"
	}

	return envName
}

// ReadProjectURLState reads the current url values straight from the project
// config file (not the merged Config), tracking whether each key is present so
// a later restore can be exact. A missing file is not an error; it yields
// HasFile=false.
func ReadProjectURLState(configPath, envName string) (ConfigURLState, error) {
	_, root, err := loadConfigDoc(configPath)
	if os.IsNotExist(err) {
		return ConfigURLState{}, nil
	}
	if err != nil {
		return ConfigURLState{}, err
	}

	state := ConfigURLState{HasFile: true}

	if node := configMapValue(root, "url"); node != nil {
		state.RootURL, state.HasRoot = node.Value, true
	}

	if envURL := envURLNode(root, envName); envURL != nil {
		state.EnvURL, state.HasEnv = envURL.Value, true
	}

	return state, nil
}

// SetProjectURL points the project config's environment (or top-level) url at
// url in place, preserving comments, ordering and unknown keys.
func SetProjectURL(configPath, envName, url string) error {
	doc, root, err := loadConfigDoc(configPath)
	if err != nil {
		return err
	}

	env := envNode(root, envName)

	if env == nil || configMapValue(root, "url") != nil {
		setConfigURLValue(root, url)
	}

	if env != nil {
		setConfigURLValue(env, url)
	}

	return writeConfigDoc(configPath, doc)
}

// RestoreProjectURL restores the url values captured in prev; previously
// absent keys are removed again.
func RestoreProjectURL(configPath, envName string, prev ConfigURLState) error {
	doc, root, err := loadConfigDoc(configPath)
	if err != nil {
		return err
	}

	if prev.HasRoot {
		setConfigURLValue(root, prev.RootURL)
	} else {
		removeConfigMapKey(root, "url")
	}

	if env := envNode(root, envName); env != nil {
		if prev.HasEnv {
			setConfigURLValue(env, prev.EnvURL)
		} else {
			removeConfigMapKey(env, "url")
		}
	}

	return writeConfigDoc(configPath, doc)
}

// envNode returns the environments.<env> mapping node, or nil.
func envNode(root *yaml.Node, envName string) *yaml.Node {
	environments := configMapValue(root, "environments")
	if environments == nil || environments.Kind != yaml.MappingNode {
		return nil
	}

	env := configMapValue(environments, urlEnvKey(envName))
	if env == nil || env.Kind != yaml.MappingNode {
		return nil
	}

	return env
}

// envURLNode returns the environments.<env>.url value node, or nil.
func envURLNode(root *yaml.Node, envName string) *yaml.Node {
	env := envNode(root, envName)
	if env == nil {
		return nil
	}

	return configMapValue(env, "url")
}

func loadConfigDoc(path string) (*yaml.Node, *yaml.Node, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("%s does not contain a YAML mapping", path)
	}

	return &doc, doc.Content[0], nil
}

func writeConfigDoc(path string, doc *yaml.Node) error {
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}

	return os.WriteFile(path, out, 0o644)
}

// configMapValue returns the value node for key in a mapping node, or nil.
func configMapValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}

	return nil
}

// setConfigURLValue updates the url key's value in a mapping node, appending
// the pair when the key is missing.
func setConfigURLValue(mapping *yaml.Node, value string) {
	if node := configMapValue(mapping, "url"); node != nil {
		node.SetString(value)
		return
	}

	keyNode := &yaml.Node{}
	keyNode.SetString("url")
	valueNode := &yaml.Node{}
	valueNode.SetString(value)

	mapping.Content = append(mapping.Content, keyNode, valueNode)
}

// removeConfigMapKey deletes key (and its value) from a mapping node.
func removeConfigMapKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}
