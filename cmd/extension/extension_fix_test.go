package extension

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionFixNonGitGuardAndSwCliRun(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", t.TempDir())
	fakeShopwareVersions(t)
	resetCommandFlags(t, extensionFixCmd)

	t.Run("non-git dir is rejected", func(t *testing.T) {
		err := runExtension(t, "fix", writePluginFixture(t))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a git repository")
	})

	t.Run("allow-non-git with sw-cli succeeds", func(t *testing.T) {
		require.NoError(t, runExtension(t, "fix", "--allow-non-git", "--only", "sw-cli", writePluginFixture(t)))
	})

	t.Run("unknown tool errors", func(t *testing.T) {
		err := runExtension(t, "fix", "--allow-non-git", "--only", "nonexistent", writePluginFixture(t))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found, possible tools")
	})
}
