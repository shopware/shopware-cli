package extension

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionBuildPluginWithoutAssets(t *testing.T) {
	t.Setenv("SHOPWARE_PROJECT_ROOT", "")

	// A plugin without admin or storefront entrypoints returns before any
	// asset tooling runs, so the whole RunE works offline.
	require.NoError(t, runExtension(t, "build", writePluginFixture(t)))

	err := runExtension(t, "build", filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot open extension")
}
