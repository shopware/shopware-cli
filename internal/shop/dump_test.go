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
				"email": "''",
			},
		},
		SystemConfigRewrite: "CASE WHEN `configuration_key` IN ('SwagExample.config.clientSecret') THEN '{\"_value\":null}' ELSE `configuration_value` END",
	})
	require.NoError(t, err)

	assert.Equal(t, "PROJECT", prepared.Rewrite["swag_example_token"]["access_token"])
	assert.Equal(t, "{{- faker.Internet.Email() -}}", prepared.Rewrite["swag_example_token"]["email"])
	assert.Equal(t, "''", prepared.Rewrite["customer"]["email"])
	assert.Equal(t, "{{- faker.Person.FirstName() -}}", prepared.Rewrite["customer"]["first_name"])
	assert.Contains(t, prepared.Rewrite["system_config"]["configuration_value"], "SwagExample.config.clientSecret")
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
		SystemConfigRewrite: "CASE WHEN `configuration_key` IN ('Ignored.key') THEN '{\"_value\":null}' ELSE `configuration_value` END",
	})
	require.NoError(t, err)

	assert.Equal(t, "PROJECT", prepared.Rewrite["system_config"]["configuration_value"])
	assert.Equal(t, "NULL", prepared.Rewrite["system_config"]["created_at"])
}

func TestPrepareDumpConfigIgnoresExtensionRulesWithoutAnonymize(t *testing.T) {
	prepared, err := prepareDumpConfig(nil, DumpDatabaseOptions{
		ExtensionTables: map[string]map[string]string{
			"swag_example_token": {"access_token": "''"},
		},
		SystemConfigRewrite: "CASE WHEN 1 THEN 1 END",
	})
	require.NoError(t, err)
	assert.Empty(t, prepared.Rewrite)
}

func TestSystemConfigRewriteExpression(t *testing.T) {
	expression, err := SystemConfigRewriteExpression([]string{
		"Zzz.config.token",
		"Aaa.config.secret",
		"Aaa.config.secret",
		"  ",
	})
	require.NoError(t, err)
	assert.Equal(t, "CASE WHEN `configuration_key` IN ('Aaa.config.secret', 'Zzz.config.token') THEN '{\"_value\":null}' ELSE `configuration_value` END", expression)

	expression, err = SystemConfigRewriteExpression(nil)
	require.NoError(t, err)
	assert.Empty(t, expression)

	_, err = SystemConfigRewriteExpression([]string{"bad key"})
	assert.Error(t, err)
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
