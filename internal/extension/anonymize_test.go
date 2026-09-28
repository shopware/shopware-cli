package extension

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/shopware/shopware-cli/internal/testhelper"
	"github.com/shopware/shopware-cli/logging"
)

func TestReadExtensionConfigAnonymize(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, ".shopware-extension.yml"), `
compatibility_date: "2026-01-01"
anonymize:
  tables:
    swag_example_token:
      access_token: "''"
      email: faker.Internet.Email()
  system_config:
    - " SwagExample.config.clientSecret "
`)

	cfg, err := readExtensionConfig(t.Context(), dir)
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{
		"swag_example_token": {
			"access_token": "''",
			"email":        "faker.Internet.Email()",
		},
	}, cfg.Anonymize.Tables)
	assert.Equal(t, []string{"SwagExample.config.clientSecret"}, cfg.Anonymize.SystemConfig)
}

func TestValidateAnonymize(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		message string
	}{
		{
			name: "invalid table",
			yaml: `
anonymize:
  tables:
    bad-name:
      token: "NULL"
`,
			message: `invalid table name "bad-name"`,
		},
		{
			name: "invalid column",
			yaml: `
anonymize:
  tables:
    swag_example_token:
      bad-column: "NULL"
`,
			message: `invalid column name "bad-column"`,
		},
		{
			name: "missing expression",
			yaml: `
anonymize:
  tables:
    swag_example_token:
      access_token: ""
`,
			message: "rewrite expression is required",
		},
		{
			name: "empty columns",
			yaml: `
anonymize:
  tables:
    swag_example_token: {}
`,
			message: "at least one column is required",
		},
		{
			name: "invalid system config key",
			yaml: `
anonymize:
  system_config:
    - "has space"
`,
			message: "invalid system_config key",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			testhelper.WriteFile(t, filepath.Join(dir, ".shopware-extension.yml"), "compatibility_date: \"2026-01-01\"\n"+tc.yaml)

			_, err := readExtensionConfig(t.Context(), dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
		})
	}
}

func TestCollectAnonymizationMergesExtensions(t *testing.T) {
	project := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(project, "bin", "console"), "")
	testhelper.WriteFile(t, filepath.Join(project, "composer.json"), testhelper.ComposerJSON{
		Require: map[string]string{"shopware/core": "~6.6.0"},
	}.String())

	writePlugin(t, project, "AaaPlugin", "Swag\\Aaa\\AaaPlugin", `
compatibility_date: "2026-01-01"
anonymize:
  tables:
    swag_shared:
      secret: "'from-aaa'"
    aaa_token:
      access_token: "''"
  system_config:
    - Aaa.config.secret
`)
	writePlugin(t, project, "ZzzPlugin", "Swag\\Zzz\\ZzzPlugin", `
compatibility_date: "2026-01-01"
anonymize:
  tables:
    swag_shared:
      secret: "'from-zzz'"
      extra: "NULL"
    zzz_customer:
      email: faker.Internet.Email()
  system_config:
    - Zzz.config.token
    - Aaa.config.secret
`)
	writePlugin(t, project, "QuietPlugin", "Swag\\Quiet\\QuietPlugin", `
compatibility_date: "2026-01-01"
`)
	vendorDir := filepath.Join(project, "vendor", "swag", "vendor-plugin")
	testhelper.WriteFile(t, filepath.Join(vendorDir, "composer.json"), testhelper.PluginComposer("swag/vendor-plugin", "1.2.0", `Swag\Vendor\VendorPlugin`).String())
	testhelper.WriteFile(t, filepath.Join(vendorDir, ".shopware-extension.yml"), `
compatibility_date: "2026-01-01"
anonymize:
  tables:
    swag_vendor_token:
      token: "NULL"
  system_config:
    - VendorPlugin.config.apiKey
`)
	testhelper.WriteFile(t, filepath.Join(project, "composer.lock"), testhelper.ComposerLock(
		testhelper.LockPackage{Name: "swag/vendor-plugin", Version: "1.2.0", Type: "shopware-platform-plugin"},
	))

	core, logs := observer.New(zap.InfoLevel)
	ctx := logging.WithLogger(t.Context(), zap.New(core).Sugar())

	rules := CollectAnonymization(ctx, project)

	assert.Equal(t, map[string]map[string]string{
		"swag_shared": {
			"secret": "'from-aaa'",
			"extra":  "NULL",
		},
		"aaa_token": {
			"access_token": "''",
		},
		"zzz_customer": {
			"email": "faker.Internet.Email()",
		},
		"swag_vendor_token": {
			"token": "NULL",
		},
	}, rules.Tables)
	assert.Equal(t, []string{"Aaa.config.secret", "VendorPlugin.config.apiKey", "Zzz.config.token"}, rules.SystemConfig)

	var conflict string
	for _, entry := range logs.All() {
		if entry.Level == zap.WarnLevel {
			conflict = entry.Message
		}
	}
	assert.Contains(t, conflict, "ZzzPlugin")
	assert.Contains(t, conflict, "AaaPlugin")
	assert.Contains(t, conflict, "swag_shared.secret")

	var merged string
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, "Merging anonymization rules from extensions") {
			merged = entry.Message
		}
	}
	assert.Contains(t, merged, "4 tables")
	assert.Contains(t, merged, "3 system config keys")
}

func writePlugin(t *testing.T, project, folder, class, config string) {
	t.Helper()
	dir := filepath.Join(project, "custom", "plugins", folder)
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), testhelper.PluginComposer("swag/"+folder, "1.0.0", class).String())
	testhelper.WriteFile(t, filepath.Join(dir, ".config", "shopware-extension.yml"), config)
}
