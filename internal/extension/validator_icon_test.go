package extension

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateExtensionIconDimensions(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		invalid       bool
	}{
		{"below minimum width", 83, 84, true},
		{"below minimum height", 84, 83, true},
		{"minimum", 84, 84, false},
		{"previous minimum", 112, 112, false},
		{"mixed valid dimensions", 84, 256, false},
		{"maximum", 256, 256, false},
		{"above maximum width", 257, 84, true},
		{"above maximum height", 84, 257, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			iconPath := filepath.Join(dir, "icon.png")
			require.NoError(t, createTestImageWithSize(iconPath, tt.width, tt.height))

			check := &testCheck{}
			validateExtensionIcon(&mockExtension{path: dir, iconPath: iconPath}, check)

			if tt.invalid {
				if assert.Len(t, check.Results, 1) {
					assert.Equal(t, "metadata.icon.size", check.Results[0].Identifier)
				}
			} else {
				assert.Empty(t, check.Results)
			}
		})
	}
}
