package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetContentHash_AdditionalCaches(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a source file
	srcDir := filepath.Join(tmpDir, "Resources", "app", "custom", "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "index.js"), []byte("console.log('v1')"), 0o644))

	entry := &ExtensionAssetConfigEntry{
		BasePath: tmpDir + "/",
		AdditionalCaches: []ConfigBuildZipAssetsAdditionalCache{
			{
				Path:        "Resources/public/custom",
				SourcePaths: []string{"Resources/app/custom/src"},
			},
		},
	}

	hash1, err := entry.GetContentHash()
	require.NoError(t, err)
	assert.NotEmpty(t, hash1)

	// Mutate the source file and verify hash changes
	entry.sumOfFiles = "" // reset cached hash
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "index.js"), []byte("console.log('v2')"), 0o644))

	hash2, err := entry.GetContentHash()
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash2)
}

func TestGetContentHash_AssetConfigChangeInvalidatesHash(t *testing.T) {
	tmpDir := t.TempDir()

	adminDir := filepath.Join(tmpDir, "Resources", "app", "administration", "src")
	require.NoError(t, os.MkdirAll(adminDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "main.js"), []byte("hello"), 0o644))

	entryFile := "Resources/app/administration/src/main.js"

	base := func() *ExtensionAssetConfigEntry {
		return &ExtensionAssetConfigEntry{
			BasePath: tmpDir + "/",
			Administration: ExtensionAssetConfigAdmin{
				Path:          "Resources/app/administration/src",
				EntryFilePath: &entryFile,
			},
		}
	}

	hash1, err := base().GetContentHash()
	require.NoError(t, err)

	// Toggling EnableESBuildForAdmin should change the hash
	e := base()
	e.EnableESBuildForAdmin = true
	hash2, err := e.GetContentHash()
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash2, "EnableESBuildForAdmin change should invalidate cache")

	// Toggling DisableSass should change the hash
	e = base()
	e.DisableSass = true
	hash3, err := e.GetContentHash()
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash3, "DisableSass change should invalidate cache")

	// Toggling NpmStrict should change the hash
	e = base()
	e.NpmStrict = true
	hash4, err := e.GetContentHash()
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash4, "NpmStrict change should invalidate cache")
}

func TestGetContentHash_AdditionalCaches_ConfigChangeInvalidatesHash(t *testing.T) {
	tmpDir := t.TempDir()

	srcDir := filepath.Join(tmpDir, "Resources", "app", "custom", "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "index.js"), []byte("hello"), 0o644))

	entry1 := &ExtensionAssetConfigEntry{
		BasePath: tmpDir + "/",
		AdditionalCaches: []ConfigBuildZipAssetsAdditionalCache{
			{
				Path:        "Resources/public/custom",
				SourcePaths: []string{"Resources/app/custom/src"},
			},
		},
	}

	hash1, err := entry1.GetContentHash()
	require.NoError(t, err)

	// Same source files, but different output path — hash must change
	entry2 := &ExtensionAssetConfigEntry{
		BasePath: tmpDir + "/",
		AdditionalCaches: []ConfigBuildZipAssetsAdditionalCache{
			{
				Path:        "Resources/public/other",
				SourcePaths: []string{"Resources/app/custom/src"},
			},
		},
	}

	hash2, err := entry2.GetContentHash()
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash2)
}

func TestGetContentHash_AdditionalCaches_SkipsNodeModules(t *testing.T) {
	tmpDir := t.TempDir()

	srcDir := filepath.Join(tmpDir, "Resources", "app", "custom", "src")
	nodeModDir := filepath.Join(srcDir, "node_modules", "pkg")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.MkdirAll(nodeModDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "index.js"), []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(nodeModDir, "lib.js"), []byte("lib v1"), 0o644))

	entry := &ExtensionAssetConfigEntry{
		BasePath: tmpDir + "/",
		AdditionalCaches: []ConfigBuildZipAssetsAdditionalCache{
			{
				Path:        "Resources/public/custom",
				SourcePaths: []string{"Resources/app/custom/src"},
			},
		},
	}

	hash1, err := entry.GetContentHash()
	require.NoError(t, err)

	// Mutate node_modules file — hash should NOT change
	entry.sumOfFiles = ""
	require.NoError(t, os.WriteFile(filepath.Join(nodeModDir, "lib.js"), []byte("lib v2"), 0o644))

	hash2, err := entry.GetContentHash()
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2)
}
