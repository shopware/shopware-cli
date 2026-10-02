package extension

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureAddSupportedRequiresShopware67130(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr string
	}{
		{name: "6.7.12.1 is below the floor", version: "6.7.12.1", wantErr: ">=6.7.13.0"},
		{name: "6.7.13.0 meets the floor", version: "6.7.13.0"},
		{name: "6.7.14.0 is above the floor", version: "6.7.14.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir := pluginProjectWithCore(t, tt.version)

			err := ensureAddSupported(projectDir)

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.ErrorContains(t, err, "plugin generators require Shopware")
			assert.ErrorContains(t, err, projectDir)
		})
	}
}

func TestEnsureAddSupportedWhenVersionCannotBeDetermined(t *testing.T) {
	projectDir := t.TempDir()

	err := ensureAddSupported(projectDir)

	require.Error(t, err)
	assert.ErrorContains(t, err, "cannot determine the Shopware version")
	assert.ErrorContains(t, err, ">=6.7.13.0")
}

func TestSplitPluginClass(t *testing.T) {
	t.Parallel()

	namespace, className, err := splitPluginClass(`Swag\BasicExample\SwagBasicExample`)
	require.NoError(t, err)
	assert.Equal(t, `Swag\BasicExample`, namespace)
	assert.Equal(t, "SwagBasicExample", className)
}

func TestSplitPluginClassRejectsUnnamespacedClass(t *testing.T) {
	t.Parallel()

	_, _, err := splitPluginClass("SwagBasicExample")
	assert.ErrorContains(t, err, "namespaced extra.shopware-plugin-class")
}

func pluginProjectWithCore(t *testing.T, coreVersion string) string {
	t.Helper()

	projectDir := t.TempDir()
	lock := map[string]any{
		"packages": []map[string]string{
			{"name": "shopware/core", "version": coreVersion},
		},
	}
	bytes, err := json.Marshal(lock)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "composer.lock"), bytes, 0o644))

	return projectDir
}
