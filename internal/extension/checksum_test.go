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
