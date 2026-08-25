package account

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountLogoutRemovesTokenCache(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", cacheDir)
	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "")

	tokenFile := filepath.Join(cacheDir, "shopware-api-token.json")
	require.NoError(t, os.WriteFile(tokenFile, []byte("{}"), 0o600))

	accountRootCmd.SetContext(t.Context())
	accountRootCmd.SetArgs([]string{"logout"})
	t.Cleanup(func() { accountRootCmd.SetArgs(nil) })

	require.NoError(t, accountRootCmd.Execute())
	assert.NoFileExists(t, tokenFile)

	// A second logout without a cache file is a no-op.
	require.NoError(t, accountRootCmd.Execute())
}
