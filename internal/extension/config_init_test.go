package extension

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateInitType(t *testing.T) {
	t.Parallel()

	assert.NoError(t, ValidateInitType("app"))
	assert.NoError(t, ValidateInitType("plugin"))
	assert.NoError(t, ValidateInitType("PLUGIN"))
	assert.Error(t, ValidateInitType(""))
	assert.Error(t, ValidateInitType("theme"))
	assert.Error(t, ValidateInitType("bundle"))
}

func TestDetectInitType(t *testing.T) {
	t.Parallel()

	t.Run("app via manifest", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.xml"), []byte("<manifest/>"), 0o644))
		got, err := DetectInitType(dir)
		require.NoError(t, err)
		assert.Equal(t, InitTypeApp, got)
	})

	t.Run("plugin via composer", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{}`), 0o644))
		got, err := DetectInitType(dir)
		require.NoError(t, err)
		assert.Equal(t, InitTypePlugin, got)
	})

	t.Run("empty dir", func(t *testing.T) {
		t.Parallel()
		_, err := DetectInitType(t.TempDir())
		assert.Error(t, err)
	})
}

func TestInitConfigPlugin(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"name":"acme/demo"}`), 0o644))

	path, err := InitConfig(dir, InitConfigOptions{
		Type:        InitTypePlugin,
		Name:        "Demo Plugin",
		Description: "A demo",
		Maintainer:  "Acme",
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ConfigFileName), path)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(raw)
	assert.Contains(t, content, "compatibility_date:")
	assert.Contains(t, content, "Maintainer: Acme")
	assert.Contains(t, content, "Demo Plugin")
	assert.Contains(t, content, "A demo")
	assert.Contains(t, content, "enabled: true") // assets/composer

	// Refuse overwrite without force.
	_, err = InitConfig(dir, InitConfigOptions{Type: InitTypePlugin})
	assert.ErrorContains(t, err, "already exists")

	// Force overwrite.
	_, err = InitConfig(dir, InitConfigOptions{Type: InitTypePlugin, Force: true, Name: "Other"})
	require.NoError(t, err)
	raw2, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw2), "Other")
}

func TestInitConfigApp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.xml"), []byte(`<manifest/>`), 0o644))

	path, err := InitConfig(dir, InitConfigOptions{Type: InitTypeApp})
	require.NoError(t, err)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "composer:")
	assert.Contains(t, string(raw), "enabled: false")

	cfg, err := readExtensionConfig(t.Context(), dir)
	require.NoError(t, err)
	assert.False(t, cfg.Build.Zip.Composer.Enabled)
	assert.True(t, cfg.Build.Zip.Assets.Enabled)
	assert.NotEmpty(t, cfg.CompatibilityDate)
	_ = path
}

func TestInitConfigStructureMismatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{}`), 0o644))

	_, err := InitConfig(dir, InitConfigOptions{Type: InitTypeApp})
	assert.ErrorContains(t, err, "manifest.xml")
}

func TestInitConfigAutoDetect(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.xml"), []byte(`<manifest/>`), 0o644))

	path, err := InitConfig(dir, InitConfigOptions{}) // type empty → detect
	require.NoError(t, err)
	assert.FileExists(t, path)
	assert.True(t, strings.HasSuffix(path, ConfigFileName))
}

func TestInitConfigRejectsMissingStructure(t *testing.T) {
	t.Parallel()

	_, err := InitConfig(t.TempDir(), InitConfigOptions{Type: InitTypePlugin})
	assert.Error(t, err)
}
