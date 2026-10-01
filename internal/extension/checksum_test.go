package extension

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestGenerateChecksumJSONOverwritesExistingChecksum(t *testing.T) {
	extensionDir := testhelper.ExtensionDir(t, testhelper.ComposerJSON{Name: "test/test-ext", Version: "1.0.0"})
	testhelper.WriteFile(t, filepath.Join(extensionDir, "src", "Resources", "changed.js"), "before")
	testhelper.WriteFile(t, filepath.Join(extensionDir, "src", "Resources", "removed.js"), "removed")

	mockExt := &mockExtension{
		name:       "TestExt",
		extVersion: version.Must(version.NewVersion("1.0.0")),
		config:     &Config{},
	}

	require.NoError(t, GenerateChecksumJSON(t.Context(), extensionDir, mockExt))

	firstContent, err := os.ReadFile(filepath.Join(extensionDir, "checksum.json"))
	require.NoError(t, err)

	var first ChecksumJSON
	require.NoError(t, json.Unmarshal(firstContent, &first))
	originalHash := first.Hashes["src/Resources/changed.js"]
	assert.NotEmpty(t, originalHash)
	assert.Contains(t, first.Hashes, "src/Resources/removed.js")

	require.NoError(t, os.WriteFile(filepath.Join(extensionDir, "src", "Resources", "changed.js"), []byte("after"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(extensionDir, "src", "Resources", "removed.js")))
	require.NoError(t, os.WriteFile(filepath.Join(extensionDir, "src", "Resources", "added.js"), []byte("added"), 0o644))

	require.NoError(t, GenerateChecksumJSON(t.Context(), extensionDir, mockExt))

	secondContent, err := os.ReadFile(filepath.Join(extensionDir, "checksum.json"))
	require.NoError(t, err)

	var second ChecksumJSON
	require.NoError(t, json.Unmarshal(secondContent, &second))
	assert.NotEqual(t, originalHash, second.Hashes["src/Resources/changed.js"])
	assert.NotContains(t, second.Hashes, "src/Resources/removed.js")
	assert.Contains(t, second.Hashes, "src/Resources/added.js")
}

func TestGenerateChecksumJSONFollowsSymlinkedExtensionRoot(t *testing.T) {
	extensionDir := testhelper.ExtensionDir(t, testhelper.ComposerJSON{Name: "test/test-ext", Version: "1.0.0"})
	testhelper.WriteFile(t, filepath.Join(extensionDir, "src", "Resources", "app.js"), "content")

	linkDir := filepath.Join(t.TempDir(), "test-ext")
	require.NoError(t, os.Symlink(extensionDir, linkDir))

	mockExt := &mockExtension{
		name:       "TestExt",
		extVersion: version.Must(version.NewVersion("1.0.0")),
		config:     &Config{},
	}

	require.NoError(t, GenerateChecksumJSON(t.Context(), linkDir, mockExt))

	content, err := os.ReadFile(filepath.Join(extensionDir, "checksum.json"))
	require.NoError(t, err)

	var checksums ChecksumJSON
	require.NoError(t, json.Unmarshal(content, &checksums))
	assert.Contains(t, checksums.Hashes, "composer.json")
	assert.Contains(t, checksums.Hashes, "src/Resources/app.js")
	assert.NotContains(t, checksums.Hashes, ".")
}

func TestGenerateChecksumJSONSkipsSymlinkedDirectories(t *testing.T) {
	extensionDir := testhelper.ExtensionDir(t, testhelper.ComposerJSON{Name: "test/test-ext", Version: "1.0.0"})
	testhelper.WriteFile(t, filepath.Join(extensionDir, "src", "Resources", "app.js"), "content")

	externalDir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(externalDir, "shared.txt"), "shared")
	require.NoError(t, os.Symlink(externalDir, filepath.Join(extensionDir, "src", "linked-dir")))
	require.NoError(t, os.Symlink(filepath.Join(externalDir, "shared.txt"), filepath.Join(extensionDir, "src", "linked-file.txt")))

	mockExt := &mockExtension{
		name:       "TestExt",
		extVersion: version.Must(version.NewVersion("1.0.0")),
		config:     &Config{},
	}

	require.NoError(t, GenerateChecksumJSON(t.Context(), extensionDir, mockExt))

	content, err := os.ReadFile(filepath.Join(extensionDir, "checksum.json"))
	require.NoError(t, err)

	var checksums ChecksumJSON
	require.NoError(t, json.Unmarshal(content, &checksums))
	assert.Contains(t, checksums.Hashes, "src/Resources/app.js")
	assert.Contains(t, checksums.Hashes, "src/linked-file.txt")
	assert.NotContains(t, checksums.Hashes, "src/linked-dir")
}

func TestGenerateChecksumJSONIgnoresDanglingSymlinkInSkippedPath(t *testing.T) {
	extensionDir := testhelper.ExtensionDir(t, testhelper.ComposerJSON{Name: "test/test-ext", Version: "1.0.0"})
	testhelper.WriteFile(t, filepath.Join(extensionDir, "src", "Resources", "app.js"), "content")
	require.NoError(t, os.Symlink(filepath.Join(extensionDir, "missing"), filepath.Join(extensionDir, "src", "ignored-link")))

	mockExt := &mockExtension{
		name:       "TestExt",
		extVersion: version.Must(version.NewVersion("1.0.0")),
		config:     &Config{},
	}
	mockExt.config.Build.Zip.Checksum.Ignore = []string{"src/ignored-link"}

	require.NoError(t, GenerateChecksumJSON(t.Context(), extensionDir, mockExt))

	content, err := os.ReadFile(filepath.Join(extensionDir, "checksum.json"))
	require.NoError(t, err)

	var checksums ChecksumJSON
	require.NoError(t, json.Unmarshal(content, &checksums))
	assert.Contains(t, checksums.Hashes, "src/Resources/app.js")
	assert.NotContains(t, checksums.Hashes, "src/ignored-link")
}
