package extension

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/compatibility"
)

func TestInitConfigWritesSchemaCommentAndToday(t *testing.T) {
	dir := t.TempDir()

	path, err := InitConfig(dir, false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ConfigFileName), path)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(raw)

	assert.Contains(t, content, "yaml-language-server: $schema=")
	assert.Contains(t, content, ConfigSchemaURL)
	assert.Contains(t, content, "compatibility_date: "+compatibility.TodayDate())
	// No forced build/store scaffolding.
	assert.NotContains(t, content, "build:")
	assert.NotContains(t, content, "store:")
}

func TestEmptyConfigFileUsesTodayDate(t *testing.T) {
	// Freeze "now" via compatibility.TodayDate (uses package clock).
	// TodayDate is time.Now-based; just assert format matches YYYY-MM-DD.
	today := time.Now().Format("2006-01-02")
	assert.Contains(t, EmptyConfigFile(), "compatibility_date: "+today)
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
