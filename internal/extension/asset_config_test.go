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
