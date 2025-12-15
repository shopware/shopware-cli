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
