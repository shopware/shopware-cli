package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionGetChangelogLanguageSelection(t *testing.T) {
	resetCommandFlags(t, extensionChangelogCmd)

	dir := writePluginFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("# 1.0.0\n- English change\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CHANGELOG_de-DE.md"), []byte("# 1.0.0\n- Deutsche Aenderung\n"), 0o644))

	require.NoError(t, runExtension(t, "get-changelog", dir))
	require.NoError(t, runExtension(t, "get-changelog", "--language", "de-DE", dir))
	// Comma-separated languages act as fallbacks.
	require.NoError(t, runExtension(t, "get-changelog", "--language", "fr-FR,de-DE", dir))

	err := runExtension(t, "get-changelog", "--language", "xx-XX", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changelog for language xx-XX not found")
}
