package shop

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/mysqldump"
)

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
