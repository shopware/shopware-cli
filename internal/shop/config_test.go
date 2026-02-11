package shop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/compatibility"
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

	config, err := ReadConfig(context.Background(), stagingFilePath, false)
	assert.NoError(t, err)

	assert.NotNil(t, config.ConfigDump.Where)

	assert.NoError(t, os.RemoveAll(tmpDir))
}

func TestReadConfigCompatibilityDateValidation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".shopware-project.yml")
	content := []byte(`
url: https://example.com
compatibility_date: 2026-13-11
`)

	assert.NoError(t, os.WriteFile(configPath, content, 0o644))

	_, err := ReadConfig(context.Background(), configPath, false)
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

	cfg, err := ReadConfig(context.Background(), configPath, true)
	assert.NoError(t, err)
	assert.Equal(t, compatibility.DefaultDate(), cfg.CompatibilityDate)
	assert.NoError(t, compatibility.ValidateDate(cfg.CompatibilityDate))
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

	t.Run("override existing column rewrites", func(t *testing.T) {
		config := &ConfigDump{
			Rewrite: map[string]map[string]string{
				"customer": {
					"first_name": "my_custom_rewrite",
					"last_name":  "another_custom_rewrite",
				},
			},
		}

		config.EnableAnonymization()

		// Anonymization should override existing rewrites
		customerRewrites := config.Rewrite["customer"]
		assert.Equal(t, "faker.Person.FirstName()", customerRewrites["first_name"])
		assert.Equal(t, "faker.Person.LastName()", customerRewrites["last_name"])
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

	t.Run("multiple calls append duplicates", func(t *testing.T) {
		config := &ConfigDump{}
		config.EnableClean()
		firstCallLength := len(config.NoData)

		config.EnableClean()

		// Second call will append again (duplicates)
		assert.Len(t, config.NoData, firstCallLength*2)
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
