package extension

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionValidateRejectsInvalidReporterAndCheckAgainst(t *testing.T) {
	resetCommandFlags(t, extensionValidateCmd)

	err := runExtension(t, "validate", "--reporter", "bogus", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid reporter format")

	// The bogus reporter value persists on the shared command, so reset
	// before checking the mode validation.
	resetFlagsNow(extensionValidateCmd)
	err = runExtension(t, "validate", "--check-against", "middle", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mode")

	// Valid reporters pass their check; the bad mode still stops PreRunE
	// before RunE, which would need network access.
	for _, reporter := range []string{"summary", "json", "github", "gitlab", "junit", "markdown"} {
		err = runExtension(t, "validate", "--reporter", reporter, "--check-against", "middle", t.TempDir())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid mode")
	}
}

func TestExtensionValidateRunsSwCliChecks(t *testing.T) {
	fakeShopwareVersions(t)
	resetCommandFlags(t, extensionValidateCmd)

	// The default run (no --full) executes only the built-in sw-cli checks,
	// which flag the minimal fixture and turn into the exit-code contract
	// CI pipelines rely on.
	t.Run("folder input", func(t *testing.T) {
		err := runExtension(t, "validate", writePluginFixture(t))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "found errors")
	})

	// Stores receive zips rather than folders, so release pipelines exercise
	// the zip input branch.
	t.Run("zip input", func(t *testing.T) {
		err := runExtension(t, "validate", buildExtensionZip(t, writePluginFixture(t)))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "found errors")
	})
}
