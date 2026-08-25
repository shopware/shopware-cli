package account

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Both commands resolve the extension before any Account API use, so these
// error paths run fully offline with the services container left nil.

func TestAccountProducerExtensionInfoPullRejectsInvalidExtension(t *testing.T) {
	t.Run("empty dir", func(t *testing.T) {
		accountRootCmd.SetContext(t.Context())
		accountRootCmd.SetArgs([]string{"producer", "extension", "info", "pull", t.TempDir()})
		t.Cleanup(func() { accountRootCmd.SetArgs(nil) })

		err := accountRootCmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot open extension")
		assert.Contains(t, err.Error(), "unknown extension type")
	})

	t.Run("shopware 5 plugin", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.xml"), []byte("<plugin/>"), 0o644))

		accountRootCmd.SetContext(t.Context())
		accountRootCmd.SetArgs([]string{"producer", "extension", "info", "pull", dir})
		t.Cleanup(func() { accountRootCmd.SetArgs(nil) })

		err := accountRootCmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "shopware 5 is not supported")
	})
}

func TestAccountProducerExtensionInfoPushPathErrors(t *testing.T) {
	t.Run("nonexistent path", func(t *testing.T) {
		accountRootCmd.SetContext(t.Context())
		accountRootCmd.SetArgs([]string{"producer", "extension", "info", "push", filepath.Join(t.TempDir(), "missing.zip")})
		t.Cleanup(func() { accountRootCmd.SetArgs(nil) })

		err := accountRootCmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot open file")
	})

	t.Run("empty dir", func(t *testing.T) {
		accountRootCmd.SetContext(t.Context())
		accountRootCmd.SetArgs([]string{"producer", "extension", "info", "push", t.TempDir()})
		t.Cleanup(func() { accountRootCmd.SetArgs(nil) })

		err := accountRootCmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot open extension")
	})
}
