package extension

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/shopware/shopware-cli/internal/compatibility"
	"github.com/shopware/shopware-cli/logging"
)

func TestInitConfigWritesSchemaCommentAndToday(t *testing.T) {
	dir := t.TempDir()

	path, err := InitConfig(t.Context(), dir, false)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".config/shopware-extension.yml"), path)

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

	_, err := InitConfig(t.Context(), dir, false)
	require.NoError(t, err)

	_, err = InitConfig(t.Context(), dir, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	_, err = InitConfig(t.Context(), dir, true)
	require.NoError(t, err)
}

func TestConfigExists(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, ConfigExists(t.Context(), dir))
	assert.Empty(t, ConfigPath(t.Context(), dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ConfigLocations[2]), []byte("{}\n"), 0o644))
	assert.True(t, ConfigExists(t.Context(), dir))
	assert.Equal(t, filepath.Join(dir, ConfigLocations[2]), ConfigPath(t.Context(), dir))
}

func TestConfigPathPriorityAndLogs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	core, logs := observer.New(zap.WarnLevel)
	ctx := logging.WithLogger(t.Context(), zap.New(core).Sugar())

	configContent := []byte(`
compatibility_date: "2026-01-01"
`)

	recommendedPath := filepath.Join(tmpDir, ".config/shopware-extension.yml")
	firstFallbackPath := filepath.Join(tmpDir, ".shopware-extension.yml")
	secondFallbackPath := filepath.Join(tmpDir, ".shopware-extension.yaml")

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".config"), 0o755))

	require.NoError(t, os.WriteFile(recommendedPath, configContent, 0o644))
	require.NoError(t, os.WriteFile(firstFallbackPath, configContent, 0o644))
	require.NoError(t, os.WriteFile(secondFallbackPath, configContent, 0o644))

	assert.Equal(t, recommendedPath, ConfigPath(ctx, tmpDir))
	logEntries := logs.TakeAll()
	assert.Len(t, logEntries, 2)
	assert.Contains(t, logEntries[0].Message, firstFallbackPath)
	assert.Contains(t, logEntries[1].Message, secondFallbackPath)

	require.NoError(t, os.Remove(recommendedPath))
	assert.Equal(t, firstFallbackPath, ConfigPath(ctx, tmpDir))
	logEntries = logs.TakeAll()
	assert.Len(t, logEntries, 1)
	assert.Contains(t, logEntries[0].Message, secondFallbackPath)

	require.NoError(t, os.Remove(firstFallbackPath))
	assert.Equal(t, secondFallbackPath, ConfigPath(ctx, tmpDir))
	logEntries = logs.TakeAll()
	assert.Empty(t, logEntries)

	// if no config file exists, it should return an empty string
	require.NoError(t, os.Remove(secondFallbackPath))
	assert.Equal(t, "", ConfigPath(ctx, tmpDir))
	logEntries = logs.TakeAll()
	assert.Empty(t, logEntries)
}
