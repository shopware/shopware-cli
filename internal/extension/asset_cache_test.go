package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateDiskCache points the default cache at a temp dir and disables the
// GitHub Actions cache backend so the test exercises the local disk cache.
func isolateDiskCache(t *testing.T) {
	t.Helper()
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_WORKFLOW", "")
}

// newAdditionalCacheEntry builds an extension entry whose only cached artifact
// is an additional_caches output path, so the cache round-trip can be tested
// without running a real asset build.
func newAdditionalCacheEntry(t *testing.T, sourceContent string) (*ExtensionAssetConfigEntry, string) {
	t.Helper()
	basePath := t.TempDir() + "/"

	srcDir := filepath.Join(basePath, "Resources", "app", "custom", "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "index.js"), []byte(sourceContent), 0o644))

	outputDir := filepath.Join(basePath, "Resources", "public", "custom")
	require.NoError(t, os.MkdirAll(outputDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outputDir, "compiled.js"), []byte("BUILD_ARTIFACT"), 0o644))

	entry := &ExtensionAssetConfigEntry{
		BasePath:      basePath,
		TechnicalName: "TestExtension",
		AdditionalCaches: []ConfigBuildZipAssetsAdditionalCache{
			{
				Path:        "Resources/public/custom",
				SourcePaths: []string{"Resources/app/custom/src"},
			},
		},
	}

	return entry, outputDir
}

func newShopwareConstraint(t *testing.T) *version.Constraints {
	t.Helper()
	constraint, err := version.NewConstraint("~6.6.0")
	require.NoError(t, err)
	return &constraint
}

func TestAssetCache_StoreAndRestoreWhenEnabled(t *testing.T) {
	isolateDiskCache(t)

	entry, outputDir := newAdditionalCacheEntry(t, "console.log('v1')")
	assetCfg := AssetBuildConfig{
		EnableAssetCaching: true,
		ShopwareVersion:    newShopwareConstraint(t),
	}
	sources := ExtensionAssetConfig{"TestExtension": entry}

	require.NoError(t, storeAssetCaches(t.Context(), sources, assetCfg))

	// Wipe the output, then restore it from cache.
	require.NoError(t, os.RemoveAll(outputDir))
	require.NoError(t, restoreAssetCaches(t.Context(), sources, assetCfg))

	restored, err := os.ReadFile(filepath.Join(outputDir, "compiled.js"))
	require.NoError(t, err, "output should be restored from cache")
	assert.Equal(t, "BUILD_ARTIFACT", string(restored))
}

func TestAssetCache_DisabledIsNoOp(t *testing.T) {
	isolateDiskCache(t)

	entry, outputDir := newAdditionalCacheEntry(t, "console.log('v1')")
	assetCfg := AssetBuildConfig{
		EnableAssetCaching: false,
		ShopwareVersion:    newShopwareConstraint(t),
	}
	sources := ExtensionAssetConfig{"TestExtension": entry}

	// Storing with caching disabled must not write anything.
	require.NoError(t, storeAssetCaches(t.Context(), sources, assetCfg))

	require.NoError(t, os.RemoveAll(outputDir))
	require.NoError(t, restoreAssetCaches(t.Context(), sources, assetCfg))

	_, err := os.Stat(filepath.Join(outputDir, "compiled.js"))
	assert.True(t, os.IsNotExist(err), "nothing should be cached or restored when disabled")
}

func TestAssetCache_SourceChangeBustsKey(t *testing.T) {
	isolateDiskCache(t)

	assetCfg := AssetBuildConfig{
		EnableAssetCaching: true,
		ShopwareVersion:    newShopwareConstraint(t),
	}

	// Store cache for an extension built from source "v1".
	v1Entry, _ := newAdditionalCacheEntry(t, "console.log('v1')")
	require.NoError(t, storeAssetCaches(t.Context(), ExtensionAssetConfig{"TestExtension": v1Entry}, assetCfg))

	// A different extension with changed source "v2" must miss the cache,
	// so its output is left untouched (not overwritten by the v1 artifact).
	v2Entry, v2Output := newAdditionalCacheEntry(t, "console.log('v2')")
	require.NoError(t, os.WriteFile(filepath.Join(v2Output, "compiled.js"), []byte("LOCAL_V2"), 0o644))

	require.NoError(t, restoreAssetCaches(t.Context(), ExtensionAssetConfig{"TestExtension": v2Entry}, assetCfg))

	content, err := os.ReadFile(filepath.Join(v2Output, "compiled.js"))
	require.NoError(t, err)
	assert.Equal(t, "LOCAL_V2", string(content), "changed source must not restore the stale cached artifact")
}
