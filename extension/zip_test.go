package extension

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetermineMinVersion(t *testing.T) {
	constraint, _ := version.NewConstraint("~6.5.0")

	matchingVersion := getMinMatchingVersion(&constraint, []string{"6.4.0.0", "6.5.0.0-rc1", "6.5.0.0"})
	assert.Equal(t, "6.5.0.0", matchingVersion)
	matchingVersion = getMinMatchingVersion(&constraint, []string{"6.4.0.0", "6.5.0.0-rc1"})
	assert.Equal(t, "6.5.0.0-rc1", matchingVersion)
	matchingVersion = getMinMatchingVersion(&constraint, []string{"6.5.0.0-rc1", "6.4.0.0"})
	assert.Equal(t, "6.5.0.0-rc1", matchingVersion)

	matchingVersion = getMinMatchingVersion(&constraint, []string{"1.0.0", "2.0.0"})
	assert.Equal(t, DevVersionNumber, matchingVersion)

	matchingVersion = getMinMatchingVersion(&constraint, []string{"6.5.0.0-rc1", "abc", "6.4.0.0"})
	assert.Equal(t, "6.5.0.0-rc1", matchingVersion)
}

func TestChecksumFile(t *testing.T) {
	// Create a temporary test file with known content
	tempDir := t.TempDir()
	testFilePath := filepath.Join(tempDir, "test.txt")
	testContent := "test content for checksum calculation"

	err := os.WriteFile(testFilePath, []byte(testContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Calculate checksum
	checksum, err := ChecksumFile(testFilePath)
	require.NoError(t, err, "ChecksumFile should not return an error")

	// The checksum value will depend on the XXH128 implementation
	// We just verify it's not empty and has the expected format (32 characters hex string)
	assert.Len(t, checksum, 32, "XXH128 checksum should be 32 characters long when hex encoded")

	// Create a second file with the same content - should have the same checksum
	testFilePath2 := filepath.Join(tempDir, "test2.txt")
	err = os.WriteFile(testFilePath2, []byte(testContent), 0644)
	require.NoError(t, err, "Failed to create second test file")

	checksum2, err := ChecksumFile(testFilePath2)
	require.NoError(t, err, "ChecksumFile should not return an error for second file")

	// Same content should have the same checksum
	assert.Equal(t, checksum, checksum2, "Same content should produce the same checksum")

	// Modify the second file and verify checksum changes
	err = os.WriteFile(testFilePath2, []byte(testContent+" modified"), 0644)
	require.NoError(t, err, "Failed to modify test file")

	checksumModified, err := ChecksumFile(testFilePath2)
	require.NoError(t, err, "ChecksumFile should not return an error for modified file")

	// Modified content should have a different checksum
	assert.NotEqual(t, checksum, checksumModified, "Modified content should produce a different checksum")
}

func TestGenerateChecksumJSON(t *testing.T) {
	// Create a temporary test directory with mock extension structure
	tempDir := t.TempDir()
	extensionDir := filepath.Join(tempDir, "TestExt")
	srcDir := filepath.Join(extensionDir, "src")
	resourcesDir := filepath.Join(srcDir, "Resources")

	// Create directories
	require.NoError(t, os.MkdirAll(resourcesDir, 0755), "Failed to create test directories")

	// Create test files
	testFiles := map[string]string{
		filepath.Join(extensionDir, "composer.json"): `{"name": "test/test-ext", "version": "1.0.0"}`,
		filepath.Join(resourcesDir, "test.txt"):      "test content",
		filepath.Join(resourcesDir, "config.js"):     "console.log('test');",
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err, "Failed to create test file: "+path)
	}

	// Create vendor and node_modules directories with files that should be ignored
	vendorDir := filepath.Join(extensionDir, "vendor")
	nodeModulesDir := filepath.Join(resourcesDir, "node_modules")

	require.NoError(t, os.MkdirAll(vendorDir, 0755), "Failed to create vendor directory")
	require.NoError(t, os.MkdirAll(nodeModulesDir, 0755), "Failed to create node_modules directory")

	require.NoError(t, os.WriteFile(filepath.Join(vendorDir, "vendor.txt"), []byte("vendor file"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(nodeModulesDir, "module.js"), []byte("module file"), 0644))

	// Create a mock extension
	mockExt := &mockExtension{
		name:       "TestExt",
		extVersion: version.Must(version.NewVersion("1.0.0")),
		config:     &Config{},
	}

	// Generate checksum.json
	err := GenerateChecksumJSON(t.Context(), extensionDir, mockExt)
	require.NoError(t, err, "GenerateChecksumJSON should not return an error")

	// Verify checksum.json was created
	checksumPath := filepath.Join(extensionDir, "checksum.json")
	assert.FileExists(t, checksumPath, "checksum.json should exist")

	// Read and parse the checksum.json
	content, err := os.ReadFile(checksumPath)
	require.NoError(t, err, "Failed to read checksum.json")

	// Verify basic content structure
	// We're not checking specific hash values as they might change with xxh128 implementation
	assert.Contains(t, string(content), `"algorithm":"xxh128"`, "Algorithm should be xxh128")
	assert.Contains(t, string(content), `"extensionVersion":"1.0.0"`, "Extension version should be 1.0.0")

	// Verify files are included
	assert.Contains(t, string(content), `"composer.json"`, "composer.json should be in the checksum list")
	assert.Contains(t, string(content), `"src/Resources/test.txt"`, "src/Resources/test.txt should be in the checksum list")
	assert.Contains(t, string(content), `"src/Resources/config.js"`, "src/Resources/config.js should be in the checksum list")

	// Verify vendor and node_modules are excluded
	assert.NotContains(t, string(content), "vendor.txt", "vendor files should be excluded")
	assert.NotContains(t, string(content), "module.js", "node_modules files should be excluded")
}

func TestGenerateChecksumJSONIgnores(t *testing.T) {
	// Create a temporary test directory with mock extension structure
	tempDir := t.TempDir()
	extensionDir := filepath.Join(tempDir, "TestExt")
	srcDir := filepath.Join(extensionDir, "src")
	resourcesDir := filepath.Join(srcDir, "Resources")

	// Create directories
	require.NoError(t, os.MkdirAll(resourcesDir, 0755), "Failed to create test directories")

	// Create test files
	testFiles := map[string]string{
		filepath.Join(extensionDir, "composer.json"): `{"name": "test/test-ext", "version": "1.0.0"}`,
		filepath.Join(resourcesDir, "test.txt"):      "test content",
	}

	for path, content := range testFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err, "Failed to create test file: "+path)
	}

	mockExt := &mockExtension{
		name:       "TestExt",
		extVersion: version.Must(version.NewVersion("1.0.0")),
		config:     &Config{},
	}

	mockExt.config.Build.Zip.Checksum.Ignore = []string{
		"src/Resources/test.txt",
	}

	err := GenerateChecksumJSON(t.Context(), extensionDir, mockExt)
	require.NoError(t, err, "GenerateChecksumJSON should not return an error")

	// Verify checksum.json was created
	checksumPath := filepath.Join(extensionDir, "checksum.json")
	assert.FileExists(t, checksumPath, "checksum.json should exist")

	// Read and parse the checksum.json
	content, err := os.ReadFile(checksumPath)
	require.NoError(t, err, "Failed to read checksum.json")

	var checksum ChecksumJSON
	err = json.Unmarshal(content, &checksum)
	require.NoError(t, err, "Failed to unmarshal checksum.json")

	// Verify that the checksum.json contains the expected files
	assert.Contains(t, checksum.Hashes, "composer.json", "composer.json should be in the checksum list")
	assert.NotContains(t, checksum.Hashes, "src/Resources/test.txt", "src/Resources/test.txt should be in the checksum list")
}
