package shop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/shopware/shopware-cli/internal/compatibility"
	"github.com/shopware/shopware-cli/logging"
)

func TestConfigMerging(t *testing.T) {
	tmpDir := t.TempDir()

	t.Chdir(tmpDir)

	baseConfig := []byte(`
admin_api:
  client_id: ${SHOPWARE_CLI_CLIENT_ID}
  client_secret: ${SHOPWARE_CLI_CLIENT_SECRET}
dump:
  where:
    customer: "email LIKE '%@nuonic.de' OR email LIKE '%@xyz.com'"
  nodata:
    - promotion
`)

	stagingConfig := []byte(`
url: https://xyz.nuonic.dev
include:
  - base.yml
`)

	baseFilePath := filepath.Join(tmpDir, "base.yml")
	stagingFilePath := filepath.Join(tmpDir, "staging.yml")

	assert.NoError(t, os.WriteFile(baseFilePath, baseConfig, 0644))
	assert.NoError(t, os.WriteFile(stagingFilePath, stagingConfig, 0644))

	config, err := ReadConfig(t.Context(), stagingFilePath, false)
	assert.NoError(t, err)

	assert.NotNil(t, config.ConfigDump.Where)
}

func TestConfigPHPVersionRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := NewConfig()
	cfg.PHPVersion = "8.3"

	assert.NoError(t, WriteConfig(cfg, tmpDir))

	// A portable version, not a machine-specific executable path.
	written, err := os.ReadFile(filepath.Join(tmpDir, ".config/shopware-project.yml"))
	assert.NoError(t, err)
	assert.Contains(t, string(written), `php_version: "8.3"`)
	assert.NotContains(t, string(written), "/bin/php")

	read, err := ReadConfig(t.Context(), filepath.Join(tmpDir, ".config/shopware-project.yml"), false)
	assert.NoError(t, err)
	assert.Equal(t, "8.3", read.PHPVersion)
}

func TestConfigWithoutPHPVersionStaysBackwardCompatible(t *testing.T) {
	tmpDir := t.TempDir()

	// A config written before php_version existed must read fine and must not
	// gain the field on re-write.
	cfg := NewConfig()
	assert.NoError(t, WriteConfig(cfg, tmpDir))

	read, err := ReadConfig(t.Context(), filepath.Join(tmpDir, ".config/shopware-project.yml"), false)
	assert.NoError(t, err)
	assert.Empty(t, read.PHPVersion)

	written, err := os.ReadFile(filepath.Join(tmpDir, ".config/shopware-project.yml"))
	assert.NoError(t, err)
	assert.NotContains(t, string(written), "php_version")
	assert.NotRegexp(t, `(?m)^url:`, string(written))
	assert.NotRegexp(t, `(?m)^admin_api:`, string(written))
	assert.Contains(t, string(written), "environments:")
}

func TestConfigDeploymentOpenSearchIndexOnInstallRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := NewConfig()
	cfg.ConfigDeployment = &ConfigDeployment{
		OpenSearch: &ConfigDeploymentOpenSearch{IndexOnInstall: true},
	}

	require.NoError(t, WriteConfig(cfg, tmpDir))

	written, err := os.ReadFile(filepath.Join(tmpDir, ".shopware-project.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(written), "opensearch:\n        index-on-install: true")

	configPath := filepath.Join(tmpDir, "opensearch.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("deployment:\n  opensearch:\n    index-on-install: true\n"), 0o644))

	read, err := ReadConfig(t.Context(), configPath, false)
	require.NoError(t, err)
	require.NotNil(t, read.ConfigDeployment)
	require.NotNil(t, read.ConfigDeployment.OpenSearch)
	assert.True(t, read.ConfigDeployment.OpenSearch.IndexOnInstall)
}

func TestReadConfigCompatibilityDateValidation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".shopware-project.yml")
	content := []byte(`
url: https://example.com
compatibility_date: 2026-13-11
`)

	assert.NoError(t, os.WriteFile(configPath, content, 0o644))

	_, err := ReadConfig(t.Context(), configPath, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid compatibility_date")
}

func TestConfigCompatibilityDateHelpers(t *testing.T) {
	cfg := &Config{CompatibilityDate: "2026-02-11"}
	assert.True(t, cfg.HasCompatibilityDate())

	ok, err := cfg.IsCompatibilityDateAtLeast("2026-02-01")
	assert.NoError(t, err)
	assert.True(t, ok)

	ok, err = cfg.IsCompatibilityDateAtLeast("2026-03-01")
	assert.NoError(t, err)
	assert.False(t, ok)

	_, err = cfg.IsCompatibilityDateAtLeast("invalid")
	assert.Error(t, err)

	emptyCfg := &Config{}
	assert.False(t, emptyCfg.HasCompatibilityDate())

	ok, err = emptyCfg.IsCompatibilityDateAtLeast("2026-01-01")
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestReadConfigFallbackSetsCompatibilityDate(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".shopware-project.yml")

	cfg, err := ReadConfig(t.Context(), configPath, true)
	assert.NoError(t, err)
	assert.Equal(t, compatibility.DefaultDate(), cfg.CompatibilityDate)
	assert.NoError(t, compatibility.ValidateDate(cfg.CompatibilityDate))
}

func TestResolveEnvironment(t *testing.T) {
	t.Run("returns named environment", func(t *testing.T) {
		cfg := &Config{
			Environments: map[string]*EnvironmentConfig{
				"staging": {Type: "docker", URL: "https://staging.example.com"},
			},
		}

		env, err := cfg.ResolveEnvironment("staging")
		assert.NoError(t, err)
		assert.Equal(t, "docker", env.Type)
		assert.Equal(t, "https://staging.example.com", env.URL)
	})

	t.Run("error on missing named environment", func(t *testing.T) {
		cfg := &Config{
			Environments: map[string]*EnvironmentConfig{
				"staging": {Type: "docker"},
			},
		}

		_, err := cfg.ResolveEnvironment("production")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `environment "production" not found`)
	})

	t.Run("returns local environment when no name given", func(t *testing.T) {
		cfg := &Config{
			Environments: map[string]*EnvironmentConfig{
				"local":   {Type: "docker", URL: "http://localhost:8000"},
				"staging": {Type: "docker", URL: "https://staging.example.com"},
			},
		}

		env, err := cfg.ResolveEnvironment("")
		assert.NoError(t, err)
		assert.Equal(t, "docker", env.Type)
		assert.Equal(t, "http://localhost:8000", env.URL)
	})

	t.Run("empty name prefers environments.local over the deprecated top-level shop", func(t *testing.T) {
		cfg := &Config{
			URL:      "https://myshop.com",
			AdminApi: &ConfigAdminApi{Username: "admin"},
			Environments: map[string]*EnvironmentConfig{
				"local": {Type: "docker", URL: "http://localhost:8000"},
			},
		}

		env, err := cfg.ResolveEnvironment("")
		require.NoError(t, err)
		assert.Equal(t, "docker", env.Type)
		assert.Equal(t, "http://localhost:8000", env.URL)
		assert.Equal(t, "admin", env.AdminApi.Username)
	})

	t.Run("empty name fills unset environment values from the deprecated top-level shop", func(t *testing.T) {
		cfg := &Config{
			URL:      "https://myshop.com",
			AdminApi: &ConfigAdminApi{Username: "admin"},
			Environments: map[string]*EnvironmentConfig{
				"local": {Type: "docker"},
			},
		}

		env, err := cfg.ResolveEnvironment("")
		require.NoError(t, err)
		assert.Equal(t, "docker", env.Type)
		assert.Equal(t, "https://myshop.com", env.URL)
		assert.Equal(t, "admin", env.AdminApi.Username)
	})

	t.Run("empty name does not mutate the stored environment", func(t *testing.T) {
		cfg := &Config{
			URL:          "https://myshop.com",
			Environments: map[string]*EnvironmentConfig{"local": {Type: "docker"}},
		}

		_, err := cfg.ResolveEnvironment("")
		require.NoError(t, err)
		assert.Empty(t, cfg.Environments["local"].URL)
	})

	t.Run("synthesizes from top-level when no environments configured", func(t *testing.T) {
		cfg := &Config{
			URL: "https://myshop.com",
			AdminApi: &ConfigAdminApi{
				Username: "admin",
				Password: "shopware",
			},
		}

		env, err := cfg.ResolveEnvironment("")
		assert.NoError(t, err)
		assert.Equal(t, "local", env.Type)
		assert.Equal(t, "https://myshop.com", env.URL)
		assert.Equal(t, "admin", env.AdminApi.Username)
	})

	t.Run("synthesizes with nil admin api", func(t *testing.T) {
		cfg := &Config{}

		env, err := cfg.ResolveEnvironment("")
		assert.NoError(t, err)
		assert.Equal(t, "local", env.Type)
		assert.Nil(t, env.AdminApi)
	})

	t.Run("error on named environment with nil map", func(t *testing.T) {
		cfg := &Config{}

		_, err := cfg.ResolveEnvironment("staging")
		assert.Error(t, err)
	})

	t.Run("error on named environment entry without configuration", func(t *testing.T) {
		cfg := &Config{Environments: map[string]*EnvironmentConfig{"staging": nil}}

		_, err := cfg.ResolveEnvironment("staging")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `environment "staging" has no configuration`)
	})

	t.Run("local entry without configuration falls back to top-level", func(t *testing.T) {
		cfg := &Config{
			URL:          "https://myshop.com",
			Environments: map[string]*EnvironmentConfig{"local": nil},
		}

		env, err := cfg.ResolveEnvironment("")
		require.NoError(t, err)
		assert.Equal(t, "local", env.Type)
		assert.Equal(t, "https://myshop.com", env.URL)
	})
}

func TestWithEnvironment(t *testing.T) {
	baseConfig := func() *Config {
		return &Config{
			URL:      "https://myshop.com",
			AdminApi: &ConfigAdminApi{Username: "admin", Password: "shopware"},
			Environments: map[string]*EnvironmentConfig{
				"staging": {
					Type:     "local",
					URL:      "https://staging.example.com",
					AdminApi: &ConfigAdminApi{ClientId: "staging-id", ClientSecret: "staging-secret"},
				},
				"bare": {Type: "local"},
			},
		}
	}

	t.Run("named environment overrides url and admin api", func(t *testing.T) {
		cfg, err := baseConfig().WithEnvironment("staging")
		require.NoError(t, err)
		assert.Equal(t, "https://staging.example.com", cfg.URL)
		assert.Equal(t, "staging-id", cfg.AdminApi.ClientId)
	})

	t.Run("environment without overrides keeps base values", func(t *testing.T) {
		cfg, err := baseConfig().WithEnvironment("bare")
		require.NoError(t, err)
		assert.Equal(t, "https://myshop.com", cfg.URL)
		assert.Equal(t, "admin", cfg.AdminApi.Username)
	})

	t.Run("error on unknown environment", func(t *testing.T) {
		_, err := baseConfig().WithEnvironment("production")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `environment "production" not found`)
	})

	t.Run("error on environment entry without configuration", func(t *testing.T) {
		cfg := &Config{
			URL:          "https://myshop.com",
			Environments: map[string]*EnvironmentConfig{"staging": nil},
		}

		_, err := cfg.WithEnvironment("staging")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `environment "staging" has no configuration`)
	})

	t.Run("no name and no environments keeps config unchanged", func(t *testing.T) {
		cfg := &Config{URL: "https://myshop.com", AdminApi: &ConfigAdminApi{Username: "admin"}}

		resolved, err := cfg.WithEnvironment("")
		require.NoError(t, err)
		assert.Equal(t, "https://myshop.com", resolved.URL)
		assert.Equal(t, "admin", resolved.AdminApi.Username)
	})

	t.Run("empty name prefers environments.local over the deprecated top-level shop", func(t *testing.T) {
		cfg := &Config{
			URL:      "https://myshop.com",
			AdminApi: &ConfigAdminApi{Username: "admin"},
			Environments: map[string]*EnvironmentConfig{
				"local": {
					URL:      "http://localhost:8000",
					AdminApi: &ConfigAdminApi{Username: "local-admin"},
				},
			},
		}

		resolved, err := cfg.WithEnvironment("")
		require.NoError(t, err)
		assert.Equal(t, "http://localhost:8000", resolved.URL)
		assert.Equal(t, "local-admin", resolved.AdminApi.Username)
	})

	t.Run("empty name applies environments.local when top-level shop is unset", func(t *testing.T) {
		cfg := &Config{
			Environments: map[string]*EnvironmentConfig{
				"local": {
					URL:      "http://localhost:8000",
					AdminApi: &ConfigAdminApi{Username: "local-admin"},
				},
			},
		}

		resolved, err := cfg.WithEnvironment("")
		require.NoError(t, err)
		assert.Equal(t, "http://localhost:8000", resolved.URL)
		assert.Equal(t, "local-admin", resolved.AdminApi.Username)
	})

	t.Run("does not mutate the original config", func(t *testing.T) {
		cfg := baseConfig()

		_, err := cfg.WithEnvironment("staging")
		assert.NoError(t, err)
		assert.Equal(t, "https://myshop.com", cfg.URL)
		assert.Equal(t, "admin", cfg.AdminApi.Username)
	})
}

func TestReadConfigWithEnvironments(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".shopware-project.yml")

	content := []byte(`
url: https://example.com
compatibility_date: "2026-01-01"
environments:
  local:
    type: docker
    url: http://localhost:8000
    admin_api:
      username: admin
      password: shopware
  staging:
    type: docker
    url: https://staging.example.com
`)

	assert.NoError(t, os.WriteFile(configPath, content, 0o644))

	config, err := ReadConfig(t.Context(), configPath, false)
	assert.NoError(t, err)
	assert.Len(t, config.Environments, 2)

	local := config.Environments["local"]
	assert.Equal(t, "docker", local.Type)
	assert.Equal(t, "http://localhost:8000", local.URL)
	assert.Equal(t, "admin", local.AdminApi.Username)

	staging := config.Environments["staging"]
	assert.Equal(t, "docker", staging.Type)
	assert.Equal(t, "https://staging.example.com", staging.URL)
}

func TestSetLocalShop(t *testing.T) {
	t.Run("writes environments.local without top-level shop keys", func(t *testing.T) {
		cfg := &Config{CompatibilityDate: "2026-01-01"}
		cfg.SetLocalShop("http://127.0.0.1:8000", &ConfigAdminApi{Username: "admin", Password: "shopware"})

		assert.Empty(t, cfg.URL)
		assert.Nil(t, cfg.AdminApi)
		require.NotNil(t, cfg.Environments["local"])
		assert.Equal(t, "local", cfg.Environments["local"].Type)
		assert.Equal(t, "http://127.0.0.1:8000", cfg.Environments["local"].URL)
		assert.Equal(t, "admin", cfg.Environments["local"].AdminApi.Username)
		assert.Equal(t, "shopware", cfg.Environments["local"].AdminApi.Password)
	})

	t.Run("preserves existing local type when already present", func(t *testing.T) {
		cfg := &Config{
			Environments: map[string]*EnvironmentConfig{
				"local": {Type: "docker"},
			},
		}
		cfg.SetLocalShop("http://127.0.0.1:8000", nil)

		assert.Equal(t, "docker", cfg.Environments["local"].Type)
		assert.Equal(t, "http://127.0.0.1:8000", cfg.Environments["local"].URL)
		assert.Nil(t, cfg.Environments["local"].AdminApi)
	})
}

func TestHasDeprecatedTopLevelShop(t *testing.T) {
	assert.False(t, (&Config{}).HasDeprecatedTopLevelShop())
	assert.False(t, NewConfig().HasDeprecatedTopLevelShop())
	assert.True(t, (&Config{URL: "https://example.com"}).HasDeprecatedTopLevelShop())
	assert.True(t, (&Config{AdminApi: &ConfigAdminApi{Username: "admin"}}).HasDeprecatedTopLevelShop())
}

func TestReadConfigWarnsOnDeprecatedTopLevelShop(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".shopware-project.yml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
url: https://example.com
compatibility_date: "2026-01-01"
admin_api:
  username: admin
  password: shopware
`), 0o644))

	core, logs := observer.New(zap.WarnLevel)
	ctx := logging.WithLogger(t.Context(), zap.New(core).Sugar())

	cfg, err := ReadConfig(ctx, configPath, false)
	require.NoError(t, err)
	assert.True(t, cfg.HasDeprecatedTopLevelShop())
	require.NotEmpty(t, logs.All())
	assert.Contains(t, logs.All()[0].Message, "deprecated top-level url/admin_api")
	assert.Contains(t, logs.All()[0].Message, "environments.local")
}

func TestReadConfigDoesNotWarnOnEnvironmentsOnly(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".shopware-project.yml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
compatibility_date: "2026-01-01"
environments:
  local:
    type: local
    url: http://127.0.0.1:8000
    admin_api:
      username: admin
      password: shopware
`), 0o644))

	core, logs := observer.New(zap.WarnLevel)
	ctx := logging.WithLogger(t.Context(), zap.New(core).Sugar())

	cfg, err := ReadConfig(ctx, configPath, false)
	require.NoError(t, err)
	assert.False(t, cfg.HasDeprecatedTopLevelShop())
	assert.Empty(t, logs.All())
}

func TestWriteConfigOmitsDeprecatedTopLevelShop(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := NewConfig()
	cfg.SetLocalShop("http://127.0.0.1:8000", &ConfigAdminApi{Username: "admin", Password: "shopware"})

	require.NoError(t, WriteConfig(cfg, tmpDir))

	written, err := os.ReadFile(filepath.Join(tmpDir, ".config/shopware-project.yml"))
	require.NoError(t, err)
	assert.NotRegexp(t, `(?m)^url:`, string(written))
	assert.NotRegexp(t, `(?m)^admin_api:`, string(written))
	assert.Contains(t, string(written), "environments:")
	assert.Contains(t, string(written), "http://127.0.0.1:8000")
}

func TestWriteConfigUsesOriginalConfigPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := NewConfig()
	cfg.SetLocalShop("http://127.0.0.1:8000", &ConfigAdminApi{Username: "admin", Password: "shopware"})
	cfg.storageLocation = ".shopware-project.yml"

	require.NoError(t, WriteConfig(cfg, tmpDir))

	written, err := os.ReadFile(filepath.Join(tmpDir, ".shopware-project.yml"))
	require.NoError(t, err)
	assert.NotRegexp(t, `(?m)^url:`, string(written))
	assert.NotRegexp(t, `(?m)^admin_api:`, string(written))
	assert.Contains(t, string(written), "environments:")
	assert.Contains(t, string(written), "http://127.0.0.1:8000")
}

func TestConfigDump_EnableAnonymization(t *testing.T) {
	t.Run("empty config", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableAnonymization()

		assert.NotNil(t, config.Rewrite)
		assert.Len(t, config.Rewrite, 7)

		// Verify all tables are present
		assert.Contains(t, config.Rewrite, "customer")
		assert.Contains(t, config.Rewrite, "customer_address")
		assert.Contains(t, config.Rewrite, "log_entry")
		assert.Contains(t, config.Rewrite, "newsletter_recipient")
		assert.Contains(t, config.Rewrite, "order_address")
		assert.Contains(t, config.Rewrite, "order_customer")
		assert.Contains(t, config.Rewrite, "product_review")
	})

	t.Run("verify customer table anonymization", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableAnonymization()

		customerRewrites := config.Rewrite["customer"]
		assert.Len(t, customerRewrites, 6)
		assert.Equal(t, "faker.Person.FirstName()", customerRewrites["first_name"])
		assert.Equal(t, "faker.Person.LastName()", customerRewrites["last_name"])
		assert.Equal(t, "faker.Person.Name()", customerRewrites["company"])
		assert.Equal(t, "faker.Person.Name()", customerRewrites["title"])
		assert.Equal(t, "faker.Internet.Email()", customerRewrites["email"])
		assert.Equal(t, "faker.Internet.Ipv4()", customerRewrites["remote_address"])
	})

	t.Run("verify customer_address table anonymization", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableAnonymization()

		addressRewrites := config.Rewrite["customer_address"]
		assert.Len(t, addressRewrites, 8)
		assert.Equal(t, "faker.Person.FirstName()", addressRewrites["first_name"])
		assert.Equal(t, "faker.Person.LastName()", addressRewrites["last_name"])
		assert.Equal(t, "faker.Person.Name()", addressRewrites["company"])
		assert.Equal(t, "faker.Person.Name()", addressRewrites["title"])
		assert.Equal(t, "faker.Address.StreetAddress()", addressRewrites["street"])
		assert.Equal(t, "faker.Address.PostCode()", addressRewrites["zipcode"])
		assert.Equal(t, "faker.Address.City()", addressRewrites["city"])
		assert.Equal(t, "faker.Phone.Number()", addressRewrites["phone_number"])
	})

	t.Run("verify log_entry table anonymization", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableAnonymization()

		logRewrites := config.Rewrite["log_entry"]
		assert.Len(t, logRewrites, 1)
		assert.Equal(t, "", logRewrites["provider"])
	})

	t.Run("verify newsletter_recipient table anonymization", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableAnonymization()

		newsletterRewrites := config.Rewrite["newsletter_recipient"]
		assert.Len(t, newsletterRewrites, 4)
		assert.Equal(t, "faker.Internet.Email()", newsletterRewrites["email"])
		assert.Equal(t, "faker.Person.FirstName()", newsletterRewrites["first_name"])
		assert.Equal(t, "faker.Person.LastName()", newsletterRewrites["last_name"])
		assert.Equal(t, "faker.Address.City()", newsletterRewrites["city"])
	})

	t.Run("verify order_address table anonymization", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableAnonymization()

		orderAddressRewrites := config.Rewrite["order_address"]
		assert.Len(t, orderAddressRewrites, 8)
		assert.Equal(t, "faker.Person.FirstName()", orderAddressRewrites["first_name"])
		assert.Equal(t, "faker.Person.LastName()", orderAddressRewrites["last_name"])
		assert.Equal(t, "faker.Person.Name()", orderAddressRewrites["company"])
		assert.Equal(t, "faker.Person.Name()", orderAddressRewrites["title"])
		assert.Equal(t, "faker.Address.StreetAddress()", orderAddressRewrites["street"])
		assert.Equal(t, "faker.Address.PostCode()", orderAddressRewrites["zipcode"])
		assert.Equal(t, "faker.Address.City()", orderAddressRewrites["city"])
		assert.Equal(t, "faker.Phone.Number()", orderAddressRewrites["phone_number"])
	})

	t.Run("verify order_customer table anonymization", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableAnonymization()

		orderCustomerRewrites := config.Rewrite["order_customer"]
		assert.Len(t, orderCustomerRewrites, 6)
		assert.Equal(t, "faker.Person.FirstName()", orderCustomerRewrites["first_name"])
		assert.Equal(t, "faker.Person.LastName()", orderCustomerRewrites["last_name"])
		assert.Equal(t, "faker.Person.Name()", orderCustomerRewrites["company"])
		assert.Equal(t, "faker.Person.Name()", orderCustomerRewrites["title"])
		assert.Equal(t, "faker.Internet.Email()", orderCustomerRewrites["email"])
		assert.Equal(t, "faker.Internet.Ipv4()", orderCustomerRewrites["remote_address"])
	})

	t.Run("verify product_review table anonymization", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableAnonymization()

		productReviewRewrites := config.Rewrite["product_review"]
		assert.Len(t, productReviewRewrites, 1)
		assert.Equal(t, "faker.Internet.Email()", productReviewRewrites["email"])
	})

	t.Run("merge with existing rewrites", func(t *testing.T) {
		config := &ConfigDump{
			Rewrite: map[string]map[string]string{
				"customer": {
					"custom_field": "custom_value",
				},
				"my_custom_table": {
					"field1": "value1",
				},
			},
		}

		config.EnableAnonymization()

		// Custom table should still exist
		assert.Contains(t, config.Rewrite, "my_custom_table")
		assert.Equal(t, "value1", config.Rewrite["my_custom_table"]["field1"])

		// Customer table should have both custom and anonymization rewrites
		customerRewrites := config.Rewrite["customer"]
		assert.Equal(t, "custom_value", customerRewrites["custom_field"])
		assert.Equal(t, "faker.Person.FirstName()", customerRewrites["first_name"])
		assert.Equal(t, "faker.Person.LastName()", customerRewrites["last_name"])
		assert.Equal(t, "faker.Internet.Email()", customerRewrites["email"])
	})

	t.Run("user rewrites take precedence over defaults", func(t *testing.T) {
		config := &ConfigDump{
			Rewrite: map[string]map[string]string{
				"customer": {
					"first_name": "my_custom_rewrite",
					"last_name":  "another_custom_rewrite",
				},
			},
		}

		config.EnableAnonymization()

		// User-supplied column rewrites must not be overwritten by the defaults
		customerRewrites := config.Rewrite["customer"]
		assert.Equal(t, "my_custom_rewrite", customerRewrites["first_name"])
		assert.Equal(t, "another_custom_rewrite", customerRewrites["last_name"])
		// Columns not configured by the user still get the default
		assert.Equal(t, "faker.Internet.Email()", customerRewrites["email"])
	})

	t.Run("multiple calls are idempotent", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableAnonymization()
		firstCallResult := make(map[string]map[string]string)
		for k, v := range config.Rewrite {
			firstCallResult[k] = make(map[string]string)
			for col, val := range v {
				firstCallResult[k][col] = val
			}
		}

		config.EnableAnonymization()
		config.EnableAnonymization()

		// Should be the same after multiple calls
		assert.Equal(t, firstCallResult, config.Rewrite)
	})
}

func TestReadConfigBundles(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".shopware-project.yml")
	content := []byte(`
compatibility_date: "2024-01-01"
build:
  bundles:
    - path: src/MyBundle
    - path: src/OtherBundle
      name: CustomName
`)
	assert.NoError(t, os.WriteFile(configPath, content, 0o644))

	cfg, err := ReadConfig(t.Context(), configPath, false)
	assert.NoError(t, err)
	assert.Len(t, cfg.Build.Bundles, 2)
	assert.Equal(t, "src/MyBundle", cfg.Build.Bundles[0].Path)
	assert.Equal(t, "", cfg.Build.Bundles[0].Name)
	assert.Equal(t, "src/OtherBundle", cfg.Build.Bundles[1].Path)
	assert.Equal(t, "CustomName", cfg.Build.Bundles[1].Name)
}

func TestReadConfigDisableChecksums(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".shopware-project.yml")
	content := []byte(`
compatibility_date: "2024-01-01"
build:
  disable_checksums: true
`)
	assert.NoError(t, os.WriteFile(configPath, content, 0o644))

	cfg, err := ReadConfig(t.Context(), configPath, false)
	assert.NoError(t, err)
	assert.True(t, cfg.Build.DisableChecksums)
}

func TestReadConfigDisableChecksumDefaultsFalse(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".shopware-project.yml")
	content := []byte(`
compatibility_date: "2024-01-01"
build: {}
`)
	assert.NoError(t, os.WriteFile(configPath, content, 0o644))

	cfg, err := ReadConfig(t.Context(), configPath, false)
	assert.NoError(t, err)
	assert.False(t, cfg.Build.DisableChecksums)
}

func TestReadConfigKeepExistingChecksums(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".shopware-project.yml")
	content := []byte(`
compatibility_date: "2024-01-01"
build:
  keep_existing_checksums: true
`)
	assert.NoError(t, os.WriteFile(configPath, content, 0o644))

	cfg, err := ReadConfig(t.Context(), configPath, false)
	assert.NoError(t, err)
	assert.True(t, cfg.Build.KeepExistingChecksums)
}

func TestConfigDump_NormalizeFakerExpressions(t *testing.T) {
	t.Run("wraps bare faker expressions with delimiters", func(t *testing.T) {
		config := &ConfigDump{
			Rewrite: map[string]map[string]string{
				"customer": {
					"email":      "faker.Internet.Email()",
					"first_name": "faker.Person.FirstName()",
				},
			},
		}

		config.NormalizeFakerExpressions()

		assert.Equal(t, "{{- faker.Internet.Email() -}}", config.Rewrite["customer"]["email"])
		assert.Equal(t, "{{- faker.Person.FirstName() -}}", config.Rewrite["customer"]["first_name"])
	})

	t.Run("handles whitespace in faker expressions", func(t *testing.T) {
		config := &ConfigDump{
			Rewrite: map[string]map[string]string{
				"customer": {
					"email": "  faker.Internet.Email()  ",
				},
			},
		}

		config.NormalizeFakerExpressions()

		assert.Equal(t, "{{- faker.Internet.Email() -}}", config.Rewrite["customer"]["email"])
	})

	t.Run("does not modify already-delimited expressions", func(t *testing.T) {
		config := &ConfigDump{
			Rewrite: map[string]map[string]string{
				"customer": {
					"email": "{{- faker.Internet.Email() -}}",
				},
			},
		}

		config.NormalizeFakerExpressions()

		assert.Equal(t, "{{- faker.Internet.Email() -}}", config.Rewrite["customer"]["email"])
	})

	t.Run("does not modify non-faker values", func(t *testing.T) {
		config := &ConfigDump{
			Rewrite: map[string]map[string]string{
				"customer": {
					"email":  "'anonymous@example.com'",
					"status": "NOW()",
				},
			},
		}

		config.NormalizeFakerExpressions()

		assert.Equal(t, "'anonymous@example.com'", config.Rewrite["customer"]["email"])
		assert.Equal(t, "NOW()", config.Rewrite["customer"]["status"])
	})

	t.Run("handles nil rewrite map", func(t *testing.T) {
		config := &ConfigDump{}

		// Should not panic
		config.NormalizeFakerExpressions()

		assert.Nil(t, config.Rewrite)
	})

	t.Run("handles multiple tables", func(t *testing.T) {
		config := &ConfigDump{
			Rewrite: map[string]map[string]string{
				"customer": {
					"email": "faker.Internet.Email()",
				},
				"order_customer": {
					"first_name": "faker.Person.FirstName()",
				},
			},
		}

		config.NormalizeFakerExpressions()

		assert.Equal(t, "{{- faker.Internet.Email() -}}", config.Rewrite["customer"]["email"])
		assert.Equal(t, "{{- faker.Person.FirstName() -}}", config.Rewrite["order_customer"]["first_name"])
	})
}

func TestConfigDump_EnableClean(t *testing.T) {
	t.Run("empty config", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableClean()

		assert.NotNil(t, config.NoData)
		assert.Len(t, config.NoData, 17)

		// Verify all tables are present
		expectedTables := []string{
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

		for _, table := range expectedTables {
			assert.Contains(t, config.NoData, table)
		}
	})

	t.Run("append to existing nodata", func(t *testing.T) {
		config := &ConfigDump{
			NoData: []string{"my_custom_table", "another_table"},
		}

		config.EnableClean()

		// Custom tables should still exist
		assert.Contains(t, config.NoData, "my_custom_table")
		assert.Contains(t, config.NoData, "another_table")

		// Clean tables should be added
		assert.Contains(t, config.NoData, "cart")
		assert.Contains(t, config.NoData, "log_entry")
		assert.Contains(t, config.NoData, "version")

		// Total should be custom tables + clean tables
		assert.Len(t, config.NoData, 19)
	})

	t.Run("no duplicates when pre-existing entry overlaps with defaults", func(t *testing.T) {
		config := &ConfigDump{
			NoData: []string{"my_custom_table", "cart", "version"},
		}

		config.EnableClean()

		// Should not introduce duplicates for 'cart' and 'version'
		count := 0
		for _, table := range config.NoData {
			if table == "cart" {
				count++
			}
		}
		assert.Equal(t, 1, count, "cart should appear exactly once")

		count = 0
		for _, table := range config.NoData {
			if table == "version" {
				count++
			}
		}
		assert.Equal(t, 1, count, "version should appear exactly once")

		// Total should be custom tables (3) + remaining clean tables (15)
		assert.Len(t, config.NoData, 18)

		// Pre-existing tables should be preserved in their original positions
		assert.Equal(t, "my_custom_table", config.NoData[0])
		assert.Equal(t, "cart", config.NoData[1])
		assert.Equal(t, "version", config.NoData[2])
	})

	t.Run("verify all expected tables", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableClean()

		// Check each specific table
		assert.Contains(t, config.NoData, "cart")
		assert.Contains(t, config.NoData, "customer_recovery")
		assert.Contains(t, config.NoData, "dead_message")
		assert.Contains(t, config.NoData, "enqueue")
		assert.Contains(t, config.NoData, "messenger_messages")
		assert.Contains(t, config.NoData, "import_export_log")
		assert.Contains(t, config.NoData, "increment")
		assert.Contains(t, config.NoData, "elasticsearch_index_task")
		assert.Contains(t, config.NoData, "log_entry")
		assert.Contains(t, config.NoData, "message_queue_stats")
		assert.Contains(t, config.NoData, "notification")
		assert.Contains(t, config.NoData, "payment_token")
		assert.Contains(t, config.NoData, "refresh_token")
		assert.Contains(t, config.NoData, "version")
		assert.Contains(t, config.NoData, "version_commit")
		assert.Contains(t, config.NoData, "version_commit_data")
		assert.Contains(t, config.NoData, "webhook_event_log")
	})

	t.Run("multiple calls are idempotent", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableClean()
		firstCallLength := len(config.NoData)

		config.EnableClean()

		// Second call should not add duplicates
		assert.Len(t, config.NoData, firstCallLength)
		assert.Contains(t, config.NoData, "cart")
		assert.Contains(t, config.NoData, "version")
	})

	t.Run("does not affect other fields", func(t *testing.T) {
		config := &ConfigDump{
			Rewrite: map[string]map[string]string{
				"customer": {"email": "test"},
			},
			Ignore: []string{"ignore_table"},
			Where: map[string]string{
				"customer": "id > 100",
			},
		}

		config.EnableClean()

		// Other fields should be unchanged
		assert.NotNil(t, config.Rewrite)
		assert.Equal(t, "test", config.Rewrite["customer"]["email"])
		assert.Contains(t, config.Ignore, "ignore_table")
		assert.Equal(t, "id > 100", config.Where["customer"])

		// NoData should be populated
		assert.Len(t, config.NoData, 17)
	})

	t.Run("preserves order of existing tables", func(t *testing.T) {
		config := &ConfigDump{
			NoData: []string{"zebra", "apple", "banana"},
		}

		config.EnableClean()

		// First three should be original tables in original order
		assert.Equal(t, "zebra", config.NoData[0])
		assert.Equal(t, "apple", config.NoData[1])
		assert.Equal(t, "banana", config.NoData[2])

		// Followed by clean tables
		assert.Equal(t, "cart", config.NoData[3])
	})
}

func TestConfigBuildMJMLResolveIncludePaths(t *testing.T) {
	projectRoot := filepath.FromSlash("/abs/project")

	t.Run("returns nil when no paths configured", func(t *testing.T) {
		c := ConfigBuildMJML{}
		assert.Nil(t, c.ResolveIncludePaths(projectRoot))
	})

	t.Run("relative paths are joined with project root", func(t *testing.T) {
		c := ConfigBuildMJML{IncludePaths: []string{
			"shared/email",
			filepath.FromSlash("custom/static-plugins/Other/Resources/views/email/_includes"),
		}}
		got := c.ResolveIncludePaths(projectRoot)
		want := []string{
			filepath.Join(projectRoot, "shared/email"),
			filepath.Join(projectRoot, "custom/static-plugins/Other/Resources/views/email/_includes"),
		}
		assert.Equal(t, want, got)
	})

	t.Run("absolute paths are returned unchanged", func(t *testing.T) {
		abs := filepath.FromSlash("/somewhere/else/_includes")
		c := ConfigBuildMJML{IncludePaths: []string{abs}}
		got := c.ResolveIncludePaths(projectRoot)
		assert.Equal(t, []string{abs}, got)
	})

	t.Run("mixed entries are resolved independently", func(t *testing.T) {
		abs := filepath.FromSlash("/somewhere/else/_includes")
		c := ConfigBuildMJML{IncludePaths: []string{"shared/email", abs}}
		got := c.ResolveIncludePaths(projectRoot)
		want := []string{filepath.Join(projectRoot, "shared/email"), abs}
		assert.Equal(t, want, got)
	})
}

func TestEffectiveURL(t *testing.T) {
	assert.Empty(t, (*Config)(nil).EffectiveURL())
	assert.Empty(t, (&Config{}).EffectiveURL())
	assert.Equal(t, "https://myshop.com", (&Config{URL: "https://myshop.com"}).EffectiveURL())

	// environments.local wins over the deprecated top-level url.
	mixed := &Config{
		URL:          "http://127.0.0.1:8000",
		Environments: map[string]*EnvironmentConfig{"local": {URL: "https://my-shop.shopware.local"}},
	}
	assert.Equal(t, "https://my-shop.shopware.local", mixed.EffectiveURL())

	// An environment without a url falls back to the top-level one.
	fallback := &Config{
		URL:          "https://myshop.com",
		Environments: map[string]*EnvironmentConfig{"local": {Type: "docker"}},
	}
	assert.Equal(t, "https://myshop.com", fallback.EffectiveURL())
}
