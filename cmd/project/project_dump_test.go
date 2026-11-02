package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func newDumpFlagCommand(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{}
	cmd.Flags().String("host", "", "")
	cmd.Flags().String("port", "", "")
	cmd.Flags().String("username", "", "")
	cmd.Flags().String("password", "", "")
	cmd.Flags().Lookup("password").NoOptDefVal = passwordFlagPrompt
	cmd.Flags().String("database", "", "")
	cmd.Flags().String("ssl-mode", "", "")
	cmd.Flags().String("ssl-ca", "", "")
	cmd.Flags().String("ssl-cert", "", "")
	cmd.Flags().String("ssl-key", "", "")
	cmd.SetContext(t.Context())

	for name, value := range flags {
		require.NoError(t, cmd.Flags().Set(name, value))
	}

	return cmd
}

// chdirOutsideProject moves into an empty directory so the project lookup
// fails and the environment-only fallback is used.
func chdirOutsideProject(t *testing.T) {
	t.Helper()
	t.Setenv("PROJECT_ROOT", "")
	t.Chdir(t.TempDir())
}

func TestAssembleConnectionURIDefaults(t *testing.T) {
	chdirOutsideProject(t)
	t.Setenv("DATABASE_URL", "")

	cfg, err := assembleConnectionURI(newDumpFlagCommand(t, nil))
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:3306", cfg.Addr)
	assert.Equal(t, "root", cfg.User)
	assert.Equal(t, "root", cfg.Passwd)
	assert.Equal(t, "shopware", cfg.DBName)
}

func TestAssembleConnectionURIFlagOverrides(t *testing.T) {
	chdirOutsideProject(t)
	t.Setenv("DATABASE_URL", "")

	cfg, err := assembleConnectionURI(newDumpFlagCommand(t, map[string]string{
		"host":     "db.internal",
		"port":     "3307",
		"username": "backup",
		"password": "secret",
		"database": "shop_prod",
	}))
	require.NoError(t, err)

	assert.Equal(t, "db.internal:3307", cfg.Addr)
	assert.Equal(t, "backup", cfg.User)
	assert.Equal(t, "secret", cfg.Passwd)
	assert.Equal(t, "shop_prod", cfg.DBName)
}

func TestAssembleConnectionURIUsernameClearsPassword(t *testing.T) {
	chdirOutsideProject(t)
	t.Setenv("DATABASE_URL", "")

	cfg, err := assembleConnectionURI(newDumpFlagCommand(t, map[string]string{"username": "backup"}))
	require.NoError(t, err)

	assert.Equal(t, "backup", cfg.User)
	assert.Empty(t, cfg.Passwd)
}

func TestAnonymizationFromInstalledExtensions(t *testing.T) {
	chdirOutsideProject(t)

	tables, rules, err := anonymizationFromInstalledExtensions(t.Context())
	require.NoError(t, err)
	assert.Nil(t, tables)
	assert.Nil(t, rules)

	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "bin", "console"), nil, 0o755))
	testhelper.WriteFile(t, filepath.Join(projectRoot, "composer.json"),
		testhelper.ComposerJSON{Require: map[string]string{"shopware/core": "6.6.0"}}.String())

	pluginDir := filepath.Join(projectRoot, "custom", "plugins", "SwagExample")
	testhelper.WriteFile(t, filepath.Join(pluginDir, "composer.json"), testhelper.PluginComposer("swag/example", "1.0.0", `Swag\Example\SwagExample`).String())
	testhelper.WriteFile(t, filepath.Join(pluginDir, ".config", "shopware-extension.yml"), `
compatibility_date: "2026-01-01"
anonymize:
  tables:
    swag_example_token:
      access_token: "''"
  system_config:
    - SwagExample.config.clientSecret
`)

	t.Setenv("PROJECT_ROOT", "")
	t.Chdir(projectRoot)

	tables, rules, err = anonymizationFromInstalledExtensions(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{
		"swag_example_token": {"access_token": "''"},
	}, tables)
	assert.Equal(t, []shop.SystemConfigRule{
		{Key: "SwagExample.config.clientSecret", Omit: true},
	}, rules)
}

func TestAssembleConnectionURIDatabaseURLInsideProject(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "bin", "console"), nil, 0o755))
	testhelper.WriteFile(t, filepath.Join(projectRoot, "composer.json"),
		testhelper.ComposerJSON{Require: map[string]string{"shopware/core": "6.6.0"}}.String())
	testhelper.WriteFile(t, filepath.Join(projectRoot, ".env"), "DATABASE_URL=mysql://app:secret@db.example.com:3307/shop\n")

	t.Setenv("PROJECT_ROOT", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SHOPWARE_CLI_NO_SYMFONY_CLI", "1")
	t.Chdir(projectRoot)

	cfg, err := assembleConnectionURI(newDumpFlagCommand(t, map[string]string{"database": "other"}))
	require.NoError(t, err)

	assert.Equal(t, "db.example.com:3307", cfg.Addr)
	assert.Equal(t, "app", cfg.User)
	assert.Equal(t, "secret", cfg.Passwd)
	assert.Equal(t, "other", cfg.DBName, "flag overrides the URL database")
}

func TestAssembleConnectionURITLSFlags(t *testing.T) {
	chdirOutsideProject(t)
	t.Setenv("DATABASE_URL", "mysql://app:pw@db.internal:3306/shop")

	cfg, err := assembleConnectionURI(newDumpFlagCommand(t, nil))
	require.NoError(t, err)
	assert.Nil(t, cfg.TLS, "TLS stays off without configuration")

	cfg, err = assembleConnectionURI(newDumpFlagCommand(t, map[string]string{"ssl-mode": "REQUIRED"}))
	require.NoError(t, err)
	require.NotNil(t, cfg.TLS)
	assert.True(t, cfg.TLS.InsecureSkipVerify)

	_, err = assembleConnectionURI(newDumpFlagCommand(t, map[string]string{"ssl-mode": "sometimes"}))
	assert.ErrorContains(t, err, "invalid TLS mode")

	_, err = assembleConnectionURI(newDumpFlagCommand(t, map[string]string{"ssl-ca": filepath.Join(t.TempDir(), "missing.pem")}))
	assert.ErrorContains(t, err, "--ssl-ca")
}

func TestApplyTLSFlagsCAImpliesVerifyCA(t *testing.T) {
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	testhelper.WriteFile(t, caFile, "pem")

	dbConn := &executor.DatabaseConnection{}
	require.NoError(t, applyTLSFlags(newDumpFlagCommand(t, map[string]string{"ssl-ca": caFile}), dbConn))
	assert.Equal(t, executor.TLSModeVerifyCA, dbConn.TLS.Mode)
	assert.Equal(t, []byte("pem"), dbConn.TLS.CA)

	// An already stricter mode from DATABASE_SSL_CA is kept.
	dbConn = &executor.DatabaseConnection{TLS: executor.DatabaseTLS{Mode: executor.TLSModeVerifyIdentity}}
	require.NoError(t, applyTLSFlags(newDumpFlagCommand(t, map[string]string{"ssl-ca": caFile}), dbConn))
	assert.Equal(t, executor.TLSModeVerifyIdentity, dbConn.TLS.Mode)

	// An explicit mode wins.
	dbConn = &executor.DatabaseConnection{}
	require.NoError(t, applyTLSFlags(newDumpFlagCommand(t, map[string]string{"ssl-ca": caFile, "ssl-mode": "verify-identity"}), dbConn))
	assert.Equal(t, executor.TLSModeVerifyIdentity, dbConn.TLS.Mode)
}
