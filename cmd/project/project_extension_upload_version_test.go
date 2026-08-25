package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
)

const testAppManifestWithVersion = `<?xml version="1.0" encoding="UTF-8"?>
<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://raw.githubusercontent.com/shopware/shopware/trunk/src/Core/Framework/App/Manifest/Schema/manifest-2.0.xsd">
	<meta>
		<name>MyExampleApp</name>
		<label>Label</label>
		<description>A description</description>
		<author>Your Company Ltd.</author>
		<copyright>(c) by Your Company Ltd.</copyright>
		<version>1.2.3</version>
		<license>MIT</license>
	</meta>
</manifest>`

func TestIncreaseExtensionVersionBumpsPluginComposerVersion(t *testing.T) {
	t.Run("patch bump", func(t *testing.T) {
		dir := writeMinimalPlugin(t)
		ext, err := extension.GetExtensionByFolder(t.Context(), dir)
		require.NoError(t, err)

		require.NoError(t, increaseExtensionVersion(t.Context(), ext))

		content, err := os.ReadFile(filepath.Join(dir, "composer.json"))
		require.NoError(t, err)
		var parsed map[string]any
		require.NoError(t, json.Unmarshal(content, &parsed))
		assert.Equal(t, "1.0.1", parsed["version"])
	})

	t.Run("four-part version bumps patch and resets build", func(t *testing.T) {
		bumped, err := bumpPatchVersion("6.6.5.2")
		require.NoError(t, err)
		assert.Equal(t, "6.6.6.0", bumped)
	})

	// go-version trims the input before validating, so the manual parser has to
	// trim too; an app manifest may contain "<version> 1.2.3 </version>".
	t.Run("whitespace-padded version bumps patch", func(t *testing.T) {
		bumped, err := bumpPatchVersion(" 1.2.3 ")
		require.NoError(t, err)
		assert.Equal(t, "1.2.4", bumped)
	})

	t.Run("composer.json without version is left untouched", func(t *testing.T) {
		dir := writeMinimalPlugin(t)
		composerPath := filepath.Join(dir, "composer.json")
		content, err := os.ReadFile(composerPath)
		require.NoError(t, err)
		var parsed map[string]any
		require.NoError(t, json.Unmarshal(content, &parsed))
		delete(parsed, "version")
		noVersion, err := json.Marshal(parsed)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(composerPath, noVersion, 0o644))

		ext, err := extension.GetExtensionByFolder(t.Context(), dir)
		require.NoError(t, err)

		require.NoError(t, increaseExtensionVersion(t.Context(), ext))

		after, err := os.ReadFile(composerPath)
		require.NoError(t, err)
		assert.Equal(t, noVersion, after)
	})
}

// The XML re-encode plus manual namespace repair is the most fragile code in
// the file; a Go xml encoder behavior change would break every app manifest
// uploaded with --increase-version.
func TestIncreaseExtensionVersionRewritesAppManifestKeepingNamespaces(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.xml"), []byte(testAppManifestWithVersion), 0o644))

	ext, err := extension.GetExtensionByFolder(t.Context(), dir)
	require.NoError(t, err)
	require.Equal(t, "app", ext.GetType())

	require.NoError(t, increaseExtensionVersion(t.Context(), ext))

	content, err := os.ReadFile(filepath.Join(dir, "manifest.xml"))
	require.NoError(t, err)
	manifest := string(content)
	assert.Contains(t, manifest, "1.2.4")
	assert.Contains(t, manifest, `xmlns:xsi=`)
	assert.Contains(t, manifest, `xsi:noNamespaceSchemaLocation=`)
	assert.NotContains(t, manifest, "_xmlns")
	assert.NotContains(t, manifest, "_XMLSchema-instance")
}
