package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
