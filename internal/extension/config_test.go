package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigValidationStringListDecode(t *testing.T) {
	cfg := `
validation:
  ignore:
    - metadata.setup
    - metadata.setup.path
`

	tmpDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-extension.yaml"), []byte(cfg), 0o644))

	ext, err := readExtensionConfig(tmpDir)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(ext.Validation.Ignore))
	assert.Equal(t, "metadata.setup", ext.Validation.Ignore[0].Identifier)
	assert.Equal(t, "metadata.setup.path", ext.Validation.Ignore[1].Identifier)
}

func TestConfigValidationStringObjectDecode(t *testing.T) {
	cfg := `
validation:
  ignore:
    - identifier: metadata.setup
    - identifier: foo
      path: bar
`

	tmpDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".shopware-extension.yaml"), []byte(cfg), 0o644))

	ext, err := readExtensionConfig(tmpDir)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(ext.Validation.Ignore))
	assert.Equal(t, "metadata.setup", ext.Validation.Ignore[0].Identifier)
	assert.Equal(t, "foo", ext.Validation.Ignore[1].Identifier)
	assert.Equal(t, "bar", ext.Validation.Ignore[1].Path)
}

func TestConfigExtraBundleResolvePath(t *testing.T) {
	tests := []struct {
		name     string
		bundle   ConfigExtraBundle
		rootDir  string
		expected string
	}{
		{
			name:     "both path and name set - uses path",
			bundle:   ConfigExtraBundle{Path: "custom/path", Name: "MyBundle"},
			rootDir:  "/root",
			expected: filepath.Join("/root", "custom/path"),
		},
		{
			name:     "only path set",
			bundle:   ConfigExtraBundle{Path: "src/Bundle"},
			rootDir:  "/root",
			expected: filepath.Join("/root", "src/Bundle"),
		},
		{
			name:     "only name set - falls back to name",
			bundle:   ConfigExtraBundle{Name: "MyBundle"},
			rootDir:  "/root",
			expected: filepath.Join("/root", "MyBundle"),
		},
		{
			name:     "both empty - returns root dir",
			bundle:   ConfigExtraBundle{},
			rootDir:  "/root",
			expected: "/root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.bundle.ResolvePath(tt.rootDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfigExtraBundleResolveName(t *testing.T) {
	tests := []struct {
		name     string
		bundle   ConfigExtraBundle
		expected string
	}{
		{
			name:     "both path and name set - uses name",
			bundle:   ConfigExtraBundle{Path: "custom/path/SomeBundle", Name: "MyBundle"},
			expected: "MyBundle",
		},
		{
			name:     "only name set",
			bundle:   ConfigExtraBundle{Name: "MyBundle"},
			expected: "MyBundle",
		},
		{
			name:     "only path set - uses base of path",
			bundle:   ConfigExtraBundle{Path: "src/MyBundle"},
			expected: "MyBundle",
		},
		{
			name:     "both empty - returns empty string",
			bundle:   ConfigExtraBundle{},
			expected: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.bundle.ResolveName()
			assert.Equal(t, tt.expected, result)
		})
	}
}
