package verifier

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestConvertExtensionToToolConfigCopiesPhpstanConfig(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "SwagExample")
	testhelper.WriteFile(t, filepath.Join(pluginDir, "composer.json"), testhelper.PluginComposer("test/swag-example", "1.0.0", `SwagExample\SwagExample`).String())
	testhelper.WriteFile(t, filepath.Join(pluginDir, ".shopware-extension.yml"), "validation:\n    phpstan_config: phpstan-verifier.neon\n")
	testhelper.WriteFile(t, filepath.Join(pluginDir, "phpstan-verifier.neon"), "parameters:\n")

	ext, err := extension.GetExtensionByFolder(t.Context(), pluginDir)
	require.NoError(t, err)

	cfg := newToolConfig(ext)

	assert.Equal(t, "phpstan-verifier.neon", cfg.PhpstanConfig)
	assert.Equal(t, ext.GetPath(), cfg.RootDir)
}
