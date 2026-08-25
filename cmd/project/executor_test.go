package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTempShopwareProject creates the minimal layout findClosestShopwareProject
// and the local executor need: composer.json requiring shopware/core, a
// bin/console stub and a .env with the given DATABASE_URL.
func writeTempShopwareProject(t *testing.T, databaseURL string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin", "console"), []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"shopware/core":"~6.6.0"}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("DATABASE_URL="+databaseURL+"\n"), 0o644))
	return dir
}

func TestConnectProjectDatabaseErrors(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_NO_SYMFONY_CLI", "1")

	t.Run("outside a project", func(t *testing.T) {
		chdirOutsideProject(t)
		t.Setenv("DATABASE_URL", "")

		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())
		conn, dbConn, cleanup, err := connectProjectDatabase(cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot find Shopware project")
		assert.Nil(t, conn)
		assert.Nil(t, dbConn)
		assert.Nil(t, cleanup)
	})

	// Connection refused on loopback port 1 fails immediately, no database
	// needed; the nil returns protect the callers' deferred cleanup.
	t.Run("unreachable database", func(t *testing.T) {
		dir := writeTempShopwareProject(t, "mysql://root:root@127.0.0.1:1/shopware")
		t.Setenv("PROJECT_ROOT", dir)
		t.Setenv("DATABASE_URL", "")

		cmd := &cobra.Command{}
		cmd.SetContext(t.Context())
		conn, dbConn, cleanup, err := connectProjectDatabase(cmd)
		require.Error(t, err)
		assert.Nil(t, conn)
		assert.Nil(t, dbConn)
		assert.Nil(t, cleanup)
	})
}
