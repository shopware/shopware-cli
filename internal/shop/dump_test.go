package shop

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/mysqldump"
)

func TestPrepareDumpConfigSkipCustomerData(t *testing.T) {
	for _, fromConfig := range []bool{false, true} {
		t.Run(map[bool]string{false: "flag", true: "config"}[fromConfig], func(t *testing.T) {
			var cfg ConfigDump
			require.NoError(t, yaml.Unmarshal([]byte("skip_customer_data: true\nnodata:\n  - payone_payment_card\nignore:\n  - obsolete_table\n"), &cfg))
			assert.True(t, cfg.SkipCustomerData)
			cfg.SkipCustomerData = fromConfig
			opts := DumpDatabaseOptions{
				SkipCustomerData: !fromConfig,
				NoData:           []string{"klarna_*", "payone_payment_card"},
			}
			prepared, err := prepareDumpConfig(&cfg, opts)
			require.NoError(t, err)
			for _, table := range []string{
				"customer", "customer_address", "customer_wishlist_product",
				"order", "order_transaction_capture_refund_position", "order_line_item_download",
				"document", "document_file", "newsletter_recipient", "product_review",
				"sales_channel_tracking_customer", "sales_channel_tracking_order",
				"payone_payment_card", "klarna_*",
			} {
				assert.Contains(t, prepared.NoData, table)
			}
			for _, table := range []string{"customer_group", "customer_group_translation", "document_type", "user", "integration", "product", "sales_channel", "version"} {
				assert.NotContains(t, prepared.NoData, table)
			}
			for _, table := range []string{"dead_message", "enqueue", "messenger_messages", "import_export_log", "log_entry", "notification", "payment_token", "version_commit", "version_commit_data", "webhook_event_log"} {
				assert.NotContains(t, prepared.NoData, table)
			}
			assert.Equal(t, []string{"obsolete_table"}, prepared.Ignore)
			firstNoData := append([]string(nil), prepared.NoData...)
			prepared, err = prepareDumpConfig(prepared, opts)
			require.NoError(t, err)
			assert.ElementsMatch(t, firstNoData, prepared.NoData)
		})
	}
}

func TestPrepareDumpConfigSkipCustomerDataWithClean(t *testing.T) {
	prepared, err := prepareDumpConfig(nil, DumpDatabaseOptions{SkipCustomerData: true, Clean: true})
	require.NoError(t, err)
	assert.Contains(t, prepared.NoData, "customer")
	assert.Contains(t, prepared.NoData, "order")
	assert.Contains(t, prepared.NoData, "elasticsearch_index_task")
	assert.Contains(t, prepared.NoData, "messenger_messages")
	assert.Contains(t, prepared.NoData, "import_export_log")
}

func TestPrepareDumpConfigConditionalTables(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured bool
		opts       DumpDatabaseOptions
		excluded   bool
	}{
		{name: "plain dump"},
		{name: "clean dump", opts: DumpDatabaseOptions{Clean: true}},
		{name: "anonymized dump", opts: DumpDatabaseOptions{Anonymize: true}},
		{name: "flag", opts: DumpDatabaseOptions{SkipCustomerData: true}, excluded: true},
		{name: "config", configured: true, excluded: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg ConfigDump
			require.NoError(t, yaml.Unmarshal([]byte(`
nodata:
  - always_excluded
skip_customer_data_tables:
  - payone_payment_card
  - 'unzer_payment_*'
`), &cfg))
			cfg.SkipCustomerData = tc.configured
			prepared, err := prepareDumpConfig(&cfg, tc.opts)
			require.NoError(t, err)
			assert.Contains(t, prepared.NoData, "always_excluded")
			for _, table := range []string{"payone_payment_card", "unzer_payment_*"} {
				if tc.excluded {
					assert.Contains(t, prepared.NoData, table)
				} else {
					assert.NotContains(t, prepared.NoData, table)
				}
			}
		})
	}
}

func TestPrepareDumpConfigRejectsAnonymizeWithSkipCustomerData(t *testing.T) {
	for _, fromConfig := range []bool{false, true} {
		_, err := prepareDumpConfig(&ConfigDump{SkipCustomerData: fromConfig}, DumpDatabaseOptions{
			Anonymize: true, SkipCustomerData: !fromConfig,
		})
		require.Error(t, err)
	}
}

func TestPrepareDumpConfigNoDataWithoutSkipCustomerData(t *testing.T) {
	prepared, err := prepareDumpConfig(nil, DumpDatabaseOptions{NoData: []string{"unzer_payment_*"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"unzer_payment_*"}, prepared.NoData)
	assert.Empty(t, prepared.Ignore)
}

func TestApplyLimitOverrides(t *testing.T) {
	cfg := &ConfigDump{}

	require.NoError(t, applyLimitOverrides(cfg, []string{"order=100", "product=5"}))

	assert.Equal(t, map[string]mysqldump.TableLimit{
		"order":   {Rows: 100},
		"product": {Rows: 5},
	}, cfg.Limit)
}

func TestApplyLimitOverridesKeepsConfiguredOrderBy(t *testing.T) {
	cfg := &ConfigDump{
		Limit: map[string]mysqldump.TableLimit{
			"order": {Rows: 10, OrderBy: "`order_number` DESC"},
		},
	}

	require.NoError(t, applyLimitOverrides(cfg, []string{"order=100"}))

	assert.Equal(t, mysqldump.TableLimit{Rows: 100, OrderBy: "`order_number` DESC"}, cfg.Limit["order"])
}

func TestPrepareDumpConfigMergesExtensionAnonymization(t *testing.T) {
	cfg := &ConfigDump{
		Rewrite: map[string]map[string]string{
			"swag_example_token": {
				"access_token": "PROJECT",
			},
		},
	}

	prepared, err := prepareDumpConfig(cfg, DumpDatabaseOptions{
		Anonymize: true,
		ExtensionTables: map[string]map[string]string{
			"Swag_Example_Token": {
				"access_token": "EXTENSION",
				"Email":        "faker.Internet.Email()",
			},
			"customer": {
				"email":         "''",
				"custom_fields": "NULL",
			},
		},
		SystemConfigRules: []SystemConfigRule{
			{Key: "SwagExample.config.clientSecret", Omit: true},
			{Key: "SwagExample.config.merchantEmail", Faker: "faker.Internet.Email()"},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "PROJECT", prepared.Rewrite["swag_example_token"]["access_token"])
	assert.Equal(t, "{{- faker.Internet.Email() -}}", prepared.Rewrite["swag_example_token"]["email"])
	assert.Equal(t, "{{- faker.Internet.Email() -}}", prepared.Rewrite["customer"]["email"])
	assert.Equal(t, "NULL", prepared.Rewrite["customer"]["custom_fields"])
	assert.Equal(t, "{{- faker.Person.FirstName() -}}", prepared.Rewrite["customer"]["first_name"])
	assert.Contains(t, prepared.Rewrite["system_config"]["configuration_value"], "SwagExample.config.merchantEmail")
	assert.Contains(t, prepared.Rewrite["system_config"]["configuration_value"], "{{- faker.Internet.Email() -}}")
	assert.Equal(t, "`configuration_key` NOT IN ('SwagExample.config.clientSecret')", prepared.Where["system_config"])
	assert.Contains(t, prepared.Rewrite, "order_customer")
}

func TestPrepareDumpConfigKeepsProjectSystemConfigRewrite(t *testing.T) {
	cfg := &ConfigDump{
		Rewrite: map[string]map[string]string{
			"system_config": {
				"configuration_value": "PROJECT",
			},
		},
	}

	prepared, err := prepareDumpConfig(cfg, DumpDatabaseOptions{
		Anonymize: true,
		ExtensionTables: map[string]map[string]string{
			"system_config": {
				"configuration_value": "EXTENSION",
				"created_at":          "NULL",
			},
		},
		SystemConfigRules: []SystemConfigRule{
			{Key: "Ignored.key", HasValue: true, Value: "sandbox"},
			{Key: "SwagExample.config.secret", Omit: true},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "PROJECT", prepared.Rewrite["system_config"]["configuration_value"])
	assert.Equal(t, "NULL", prepared.Rewrite["system_config"]["created_at"])
	assert.Equal(t, "`configuration_key` NOT IN ('SwagExample.config.secret')", prepared.Where["system_config"])
}

func TestPrepareDumpConfigIgnoresExtensionRulesWithoutAnonymize(t *testing.T) {
	prepared, err := prepareDumpConfig(nil, DumpDatabaseOptions{
		ExtensionTables: map[string]map[string]string{
			"swag_example_token": {"access_token": "''"},
		},
		SystemConfigRules: []SystemConfigRule{{Key: "SwagExample.config.secret", Omit: true}},
	})
	require.NoError(t, err)
	assert.Empty(t, prepared.Rewrite)
	assert.Empty(t, prepared.Where)
}

func TestBuildSystemConfigAnonymization(t *testing.T) {
	rewrite, where, err := BuildSystemConfigAnonymization([]SystemConfigRule{
		{Key: "B.config.mode", HasValue: true, Value: "sandbox"},
		{Key: "A.config.email", Faker: "faker.Internet.Email()"},
		{Key: "C.config.secret", Omit: true},
		{Key: "D.config.on", HasValue: true, Value: false},
		{Key: "E.config.name", HasValue: true, Value: "O'Brien"},
		{Key: "C.config.secret", Omit: true},
	})
	require.NoError(t, err)
	assert.Equal(t, "`configuration_key` NOT IN ('C.config.secret')", where)
	assert.Equal(t,
		"CASE WHEN `configuration_key` = 'A.config.email' THEN '{\"_value\":\"{{- faker.Internet.Email() -}}\"}' WHEN `configuration_key` = 'B.config.mode' THEN '{\"_value\":\"sandbox\"}' WHEN `configuration_key` = 'D.config.on' THEN '{\"_value\":false}' WHEN `configuration_key` = 'E.config.name' THEN '{\"_value\":\"O''Brien\"}' ELSE `configuration_value` END",
		rewrite,
	)

	rewrite, where, err = BuildSystemConfigAnonymization(nil)
	require.NoError(t, err)
	assert.Empty(t, rewrite)
	assert.Empty(t, where)

	_, _, err = BuildSystemConfigAnonymization([]SystemConfigRule{{Key: "bad key", Omit: true}})
	assert.Error(t, err)
}

func TestPrepareDumpConfigMergesSystemConfigWhere(t *testing.T) {
	prepared, err := prepareDumpConfig(&ConfigDump{
		Where: map[string]string{"system_config": "sales_channel_id IS NULL"},
	}, DumpDatabaseOptions{
		Anonymize: true,
		SystemConfigRules: []SystemConfigRule{
			{Key: "SwagExample.config.secret", Omit: true},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "(sales_channel_id IS NULL) AND (`configuration_key` NOT IN ('SwagExample.config.secret'))", prepared.Where["system_config"])
}

func TestApplyLimitOverridesValidation(t *testing.T) {
	cases := []struct {
		override    string
		expectedErr string
	}{
		{"order", `invalid --limit "order", expected format table=rows (e.g. order=100)`},
		{"order=abc", `invalid --limit "order=abc", rows must be a positive number`},
		{"order=0", `invalid --limit "order=0", rows must be a positive number`},
		{"order=-5", `invalid --limit "order=-5", rows must be a positive number`},
	}

	for _, tc := range cases {
		t.Run(tc.override, func(t *testing.T) {
			err := applyLimitOverrides(&ConfigDump{}, []string{tc.override})
			assert.EqualError(t, err, tc.expectedErr)
		})
	}
}
