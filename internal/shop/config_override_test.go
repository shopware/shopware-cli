package shop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestLocalConfigFileName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{".shopware-project.yml", ".shopware-project.local.yml"},
		{".shopware-project.yaml", ".shopware-project.local.yaml"},
		{"/path/to/.shopware-project.yml", "/path/to/.shopware-project.local.yml"},
		{"config.yml", "config.local.yml"},
		{"config.yaml", "config.local.yaml"},
		{"noext", "noext.local"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, localConfigFileName(tt.input))
		})
	}
}

func TestLocalConfigOverrideScalar(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	baseConfig := []byte(`
url: https://example.com
compatibility_date: "2026-01-01"
admin_api:
  client_id: base_id
  client_secret: base_secret
`)

	localConfig := []byte(`
url: https://local.example.com
admin_api:
  client_id: local_id
`)

	basePath := filepath.Join(tmpDir, ".shopware-project.yml")
	localPath := filepath.Join(tmpDir, ".shopware-project.local.yml")

	assert.NoError(t, os.WriteFile(basePath, baseConfig, 0o644))
	assert.NoError(t, os.WriteFile(localPath, localConfig, 0o644))

	config, err := ReadConfig(t.Context(), basePath, false)
	assert.NoError(t, err)

	// Local value overrides base
	assert.Equal(t, "https://local.example.com", config.URL)
	// Local value overrides nested field
	assert.Equal(t, "local_id", config.AdminApi.ClientId)
	// Base value preserved when not in local
	assert.Equal(t, "base_secret", config.AdminApi.ClientSecret)
}

func TestLocalConfigOverrideSliceAppend(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	baseConfig := []byte(`
compatibility_date: "2026-01-01"
build:
  cleanup_paths:
    - var/cache
    - var/log
`)

	localConfig := []byte(`
build:
  cleanup_paths:
    - custom/path
`)

	basePath := filepath.Join(tmpDir, ".shopware-project.yml")
	localPath := filepath.Join(tmpDir, ".shopware-project.local.yml")

	assert.NoError(t, os.WriteFile(basePath, baseConfig, 0o644))
	assert.NoError(t, os.WriteFile(localPath, localConfig, 0o644))

	config, err := ReadConfig(t.Context(), basePath, false)
	assert.NoError(t, err)

	// Slices are appended by default
	assert.Len(t, config.Build.CleanupPaths, 3)
	assert.Contains(t, config.Build.CleanupPaths, "var/cache")
	assert.Contains(t, config.Build.CleanupPaths, "var/log")
	assert.Contains(t, config.Build.CleanupPaths, "custom/path")
}

func TestLocalConfigResetSlice(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	baseConfig := []byte(`
compatibility_date: "2026-01-01"
build:
  cleanup_paths:
    - var/cache
    - var/log
`)

	localConfig := []byte(`
build:
  cleanup_paths: !reset []
`)

	basePath := filepath.Join(tmpDir, ".shopware-project.yml")
	localPath := filepath.Join(tmpDir, ".shopware-project.local.yml")

	assert.NoError(t, os.WriteFile(basePath, baseConfig, 0o644))
	assert.NoError(t, os.WriteFile(localPath, localConfig, 0o644))

	config, err := ReadConfig(t.Context(), basePath, false)
	assert.NoError(t, err)

	// !reset clears the list
	assert.Empty(t, config.Build.CleanupPaths)
}

func TestLocalConfigResetSliceWithNewValues(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	baseConfig := []byte(`
compatibility_date: "2026-01-01"
build:
  cleanup_paths:
    - var/cache
    - var/log
`)

	localConfig := []byte(`
build:
  cleanup_paths: !reset
    - only/this
`)

	basePath := filepath.Join(tmpDir, ".shopware-project.yml")
	localPath := filepath.Join(tmpDir, ".shopware-project.local.yml")

	assert.NoError(t, os.WriteFile(basePath, baseConfig, 0o644))
	assert.NoError(t, os.WriteFile(localPath, localConfig, 0o644))

	config, err := ReadConfig(t.Context(), basePath, false)
	assert.NoError(t, err)

	// !reset clears the base list, then the new values are set
	assert.Equal(t, []string{"only/this"}, config.Build.CleanupPaths)
}

func TestLocalConfigOverrideMap(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	baseConfig := []byte(`
compatibility_date: "2026-01-01"
dump:
  nodata:
    - cart
    - log_entry
  where:
    customer: "id > 100"
    order: "created_at > '2024-01-01'"
`)

	localConfig := []byte(`
dump: !override
  nodata:
    - cart
`)

	basePath := filepath.Join(tmpDir, ".shopware-project.yml")
	localPath := filepath.Join(tmpDir, ".shopware-project.local.yml")

	assert.NoError(t, os.WriteFile(basePath, baseConfig, 0o644))
	assert.NoError(t, os.WriteFile(localPath, localConfig, 0o644))

	config, err := ReadConfig(t.Context(), basePath, false)
	assert.NoError(t, err)

	// !override replaces entire dump section
	assert.Equal(t, []string{"cart"}, config.ConfigDump.NoData)
	// where is gone because the entire dump was replaced
	assert.Nil(t, config.ConfigDump.Where)
}

func TestLocalConfigResetNestedField(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	baseConfig := []byte(`
compatibility_date: "2026-01-01"
dump:
  nodata:
    - cart
    - log_entry
  where:
    customer: "id > 100"
`)

	localConfig := []byte(`
dump:
  nodata: !reset []
`)

	basePath := filepath.Join(tmpDir, ".shopware-project.yml")
	localPath := filepath.Join(tmpDir, ".shopware-project.local.yml")

	assert.NoError(t, os.WriteFile(basePath, baseConfig, 0o644))
	assert.NoError(t, os.WriteFile(localPath, localConfig, 0o644))

	config, err := ReadConfig(t.Context(), basePath, false)
	assert.NoError(t, err)

	// nodata is reset to empty
	assert.Empty(t, config.ConfigDump.NoData)
	// where is still preserved (only nodata was reset)
	assert.Equal(t, "id > 100", config.ConfigDump.Where["customer"])
}

func TestLocalConfigNoLocalFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	baseConfig := []byte(`
url: https://example.com
compatibility_date: "2026-01-01"
`)

	basePath := filepath.Join(tmpDir, ".shopware-project.yml")
	assert.NoError(t, os.WriteFile(basePath, baseConfig, 0o644))

	config, err := ReadConfig(t.Context(), basePath, false)
	assert.NoError(t, err)

	assert.Equal(t, "https://example.com", config.URL)
}

func TestLocalConfigWithInclude(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	includedConfig := []byte(`
admin_api:
  client_id: included_id
  client_secret: included_secret
`)

	baseConfig := []byte(`
url: https://example.com
compatibility_date: "2026-01-01"
include:
  - included.yml
`)

	localConfig := []byte(`
url: https://local.example.com
`)

	includedPath := filepath.Join(tmpDir, "included.yml")
	basePath := filepath.Join(tmpDir, ".shopware-project.yml")
	localPath := filepath.Join(tmpDir, ".shopware-project.local.yml")

	assert.NoError(t, os.WriteFile(includedPath, includedConfig, 0o644))
	assert.NoError(t, os.WriteFile(basePath, baseConfig, 0o644))
	assert.NoError(t, os.WriteFile(localPath, localConfig, 0o644))

	config, err := ReadConfig(t.Context(), basePath, false)
	assert.NoError(t, err)

	// Local override wins
	assert.Equal(t, "https://local.example.com", config.URL)
	// Included config values are still present
	assert.Equal(t, "included_id", config.AdminApi.ClientId)
}

func TestLocalConfigEmptyLocalFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	baseConfig := []byte(`
url: https://example.com
compatibility_date: "2026-01-01"
`)

	localConfig := []byte(``)

	basePath := filepath.Join(tmpDir, ".shopware-project.yml")
	localPath := filepath.Join(tmpDir, ".shopware-project.local.yml")

	assert.NoError(t, os.WriteFile(basePath, baseConfig, 0o644))
	assert.NoError(t, os.WriteFile(localPath, localConfig, 0o644))

	config, err := ReadConfig(t.Context(), basePath, false)
	assert.NoError(t, err)

	assert.Equal(t, "https://example.com", config.URL)
}

func TestMergeMapDeepMerge(t *testing.T) {
	dst := map[string]any{
		"a": map[string]any{
			"b": "base_b",
			"c": "base_c",
		},
		"d": "base_d",
	}

	src := map[string]any{
		"a": map[string]any{
			"b": "override_b",
			"e": "new_e",
		},
		"f": "new_f",
	}

	mergeMap(dst, src, "", map[string]bool{})

	a := dst["a"].(map[string]any)
	assert.Equal(t, "override_b", a["b"])
	assert.Equal(t, "base_c", a["c"])
	assert.Equal(t, "new_e", a["e"])
	assert.Equal(t, "base_d", dst["d"])
	assert.Equal(t, "new_f", dst["f"])
}

func TestMergeMapOverridePath(t *testing.T) {
	dst := map[string]any{
		"a": map[string]any{
			"b": "base_b",
			"c": "base_c",
		},
	}

	src := map[string]any{
		"a": map[string]any{
			"b": "override_b",
		},
	}

	overridePaths := map[string]bool{"a": true}
	mergeMap(dst, src, "", overridePaths)

	// Since "a" is in overridePaths, it's fully replaced
	a := dst["a"].(map[string]any)
	assert.Equal(t, "override_b", a["b"])
	_, hasC := a["c"]
	assert.False(t, hasC, "key 'c' should not exist after !override")
}

func TestDeleteAtPath(t *testing.T) {
	m := map[string]any{
		"a": map[string]any{
			"b": "value",
			"c": "keep",
		},
		"d": "keep_d",
	}

	deleteAtPath(m, "a.b")
	a := m["a"].(map[string]any)
	_, hasB := a["b"]
	assert.False(t, hasB)
	assert.Equal(t, "keep", a["c"])
	assert.Equal(t, "keep_d", m["d"])
}

func TestCollectTaggedPaths(t *testing.T) {
	yamlContent := `
build:
  cleanup_paths: !reset
    - only/this
  exclude_extensions: !override
    - MyExtension
`

	var node yaml.Node
	assert.NoError(t, yaml.Unmarshal([]byte(yamlContent), &node))

	resetPaths := make(map[string]bool)
	overridePaths := make(map[string]bool)
	collectTaggedPaths(&node, "", resetPaths, overridePaths)

	assert.True(t, resetPaths["build.cleanup_paths"])
	assert.True(t, overridePaths["build.exclude_extensions"])
}
