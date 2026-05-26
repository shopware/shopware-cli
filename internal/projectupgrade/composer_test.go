package projectupgrade

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeJSON(t *testing.T, file string, content map[string]any) {
	t.Helper()

	data, err := json.MarshalIndent(content, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, data, 0o644))
}

func readJSON(t *testing.T, file string) map[string]any {
	t.Helper()

	raw, err := os.ReadFile(file)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestUpdateComposerJsonRewritesShopwarePackages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core":           "6.5.8.0",
			"shopware/administration": "6.5.8.0",
			"shopware/storefront":     "6.5.8.0",
			"unrelated/package":       "^1.0",
		},
	})

	require.NoError(t, UpdateComposerJson(composerJsonPath, "6.6.4.0"))

	out := readJSON(t, composerJsonPath)
	requireMap := out["require"].(map[string]any)
	assert.Equal(t, "6.6.4.0", requireMap["shopware/core"])
	assert.Equal(t, "6.6.4.0", requireMap["shopware/administration"])
	assert.Equal(t, "6.6.4.0", requireMap["shopware/storefront"])
	assert.Equal(t, "^1.0", requireMap["unrelated/package"])
	assert.NotContains(t, requireMap, "shopware/elasticsearch", "should not add packages that were not already required")
}

func TestUpdateComposerJsonSetsRCStability(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core": "6.5.8.0",
		},
	})

	require.NoError(t, UpdateComposerJson(composerJsonPath, "6.6.0.0-rc1"))
	out := readJSON(t, composerJsonPath)
	assert.Equal(t, "RC", out["minimum-stability"])
}

func TestUpdateComposerJsonClearsRCStabilityForStableTarget(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	writeJSON(t, composerJsonPath, map[string]any{
		"name":              "shopware/production",
		"minimum-stability": "RC",
		"require": map[string]any{
			"shopware/core": "6.5.8.0",
		},
	})

	require.NoError(t, UpdateComposerJson(composerJsonPath, "6.6.4.0"))
	out := readJSON(t, composerJsonPath)
	_, hasStability := out["minimum-stability"]
	assert.False(t, hasStability, "minimum-stability should be cleared for stable upgrades")
}

func TestUpdateComposerJsonRewritesSymfonyRuntimeConstraint(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core":   "6.5.8.0",
			"symfony/runtime": "^5.4|^6.0",
		},
	})

	require.NoError(t, UpdateComposerJson(composerJsonPath, "6.6.4.0"))
	out := readJSON(t, composerJsonPath)
	requireMap := out["require"].(map[string]any)
	assert.Equal(t, ">=5", requireMap["symfony/runtime"])
}
