package extension

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/system"
)

func TestExtensionConfigInitCreatesAndGuardsOverwrite(t *testing.T) {
	resetCommandFlags(t, extensionConfigInitCmd)
	dir := t.TempDir()
	configFile := filepath.Join(dir, ".shopware-extension.yml")
	// Interaction defaults to enabled on a bare context; disable it so the
	// exists-guard errors instead of opening a confirm prompt.
	ctx := system.WithInteraction(t.Context(), false)

	require.NoError(t, runExtensionCtx(t, ctx, "config", "init", dir))
	content, err := os.ReadFile(configFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "yaml-language-server")
	assert.Contains(t, string(content), "compatibility_date")

	err = runExtensionCtx(t, ctx, "config", "init", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	require.NoError(t, runExtensionCtx(t, ctx, "config", "init", "--force", dir))
}

func TestExtensionConfigSchemaPrintsValidJSON(t *testing.T) {
	out := new(bytes.Buffer)
	extensionConfigSchemaCmd.SetOut(out)
	t.Cleanup(func() { extensionConfigSchemaCmd.SetOut(nil) })

	require.NoError(t, runExtension(t, "config-schema"))
	assert.True(t, json.Valid(out.Bytes()))
	assert.Contains(t, out.String(), "$schema")
}
