package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAssetByResourceDirStorefront(t *testing.T) {
	tests := []struct {
		name            string
		buildPresent    bool
		sourcePresent   bool
		expectedID      string
		expectedMessage string
	}{
		{
			name:          "build and source present",
			buildPresent:  true,
			sourcePresent: true,
		},
		{
			name:            "build present and source missing",
			buildPresent:    true,
			expectedID:      "assets.storefront.sources_missing",
			expectedMessage: "Found storefront build files",
		},
		{
			name:            "build missing and source present",
			sourcePresent:   true,
			expectedID:      "assets.storefront.build_missing",
			expectedMessage: "Found storefront source files",
		},
		{
			name: "build and source missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceDir := t.TempDir()
			if tt.buildPresent {
				assert.NoError(t, os.MkdirAll(filepath.Join(resourceDir, "app", "storefront", "dist"), 0o755))
			}
			if tt.sourcePresent {
				sourceDir := filepath.Join(resourceDir, "app", "storefront", "src")
				assert.NoError(t, os.MkdirAll(sourceDir, 0o755))
				assert.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.js"), []byte(""), 0o644))
			}

			check := &testCheck{}
			validateAssetByResourceDir(check, resourceDir)

			if tt.expectedID == "" {
				assert.Empty(t, check.Results)
				return
			}

			if assert.Len(t, check.Results, 1) {
				assert.Equal(t, tt.expectedID, check.Results[0].Identifier)
				assert.Contains(t, check.Results[0].Message, tt.expectedMessage)
			}
		})
	}
}
