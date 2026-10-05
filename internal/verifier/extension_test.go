package verifier

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestSetupExtensionToolConfigUsesConfiguredToolDirectory(t *testing.T) {
	stubShopwareVersions(t)
	previous := toolDirectory
	t.Cleanup(func() { setToolDirectory(previous) })
	setToolDirectory(filepath.Join(t.TempDir(), "stale"))

	toolDir := filepath.Join(t.TempDir(), "tools")
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", toolDir)
	ext, err := extension.GetExtensionByFolder(t.Context(), testhelper.NewPlugin(t, "Example"))
	require.NoError(t, err)

	cfg, err := SetupExtensionToolConfig(t.Context(), "test", ext)
	require.NoError(t, err)
	assert.Equal(t, toolDir, cfg.ToolDirectory)
}
