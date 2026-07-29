package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitConfigWritesSchemaCommentOnly(t *testing.T) {
	dir := t.TempDir()

	path, err := InitConfig(dir, false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ConfigFileName), path)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, EmptyConfigFile, string(raw))
	assert.Contains(t, string(raw), "yaml-language-server: $schema=")
	assert.Contains(t, string(raw), ConfigSchemaURL)
	// Empty body — no forced keys.
	assert.NotContains(t, string(raw), "compatibility_date:")
	assert.NotContains(t, string(raw), "build:")
}

func TestInitConfigRefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()

	_, err := InitConfig(dir, false)
	require.NoError(t, err)

	_, err = InitConfig(dir, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	_, err = InitConfig(dir, true)
	require.NoError(t, err)
}

func TestConfigExists(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, ConfigExists(dir))
	assert.Empty(t, ConfigPath(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ConfigFileNameAlt), []byte("{}\n"), 0o644))
	assert.True(t, ConfigExists(dir))
	assert.Equal(t, filepath.Join(dir, ConfigFileNameAlt), ConfigPath(dir))
}
