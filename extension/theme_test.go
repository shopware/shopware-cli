package extension

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/validation"
)

func TestValidateTheme_NoThemeFile(t *testing.T) {
	tmpDir := t.TempDir()

	ext := &mockExtension{
		path: tmpDir,
	}
	// Override GetRootDir to return our test directory
	ext.rootDir = tmpDir

	check := &testCheck{}
	validateTheme(ext, check)

	// Should not add any results when theme.json doesn't exist
	assert.Empty(t, check.Results)
}

func TestValidateTheme_InvalidThemeJSON(t *testing.T) {
	tmpDir := t.TempDir()
	resourcesDir := filepath.Join(tmpDir, "Resources")
	err := os.MkdirAll(resourcesDir, 0755)
	assert.NoError(t, err)

	// Create invalid JSON file
	themeJSONPath := filepath.Join(resourcesDir, "theme.json")
	err = os.WriteFile(themeJSONPath, []byte("invalid json"), 0644)
	assert.NoError(t, err)

	ext := &mockExtension{
		path:    tmpDir,
		rootDir: tmpDir,
	}

	check := &testCheck{}
	validateTheme(ext, check)

	assert.Len(t, check.Results, 1)
	assert.Equal(t, "Resources/theme.json", check.Results[0].Path)
	assert.Equal(t, "theme.validator", check.Results[0].Identifier)
	assert.Equal(t, "Cannot decode theme.json", check.Results[0].Message)
	assert.Equal(t, validation.SeverityError, check.Results[0].Severity)
}

func TestValidateTheme_ReadError(t *testing.T) {
	tmpDir := t.TempDir()
	resourcesDir := filepath.Join(tmpDir, "Resources")
	err := os.MkdirAll(resourcesDir, 0755)
	assert.NoError(t, err)

	// Create a file with no read permissions
	themeJSONPath := filepath.Join(resourcesDir, "theme.json")
	err = os.WriteFile(themeJSONPath, []byte(`{"previewMedia": "test.png"}`), 0000)
	assert.NoError(t, err)

	ext := &mockExtension{
		path:    tmpDir,
		rootDir: tmpDir,
	}

	check := &testCheck{}
	validateTheme(ext, check)

	assert.Len(t, check.Results, 1)
	assert.Equal(t, "Resources/theme.json", check.Results[0].Path)
	assert.Equal(t, "theme.validator", check.Results[0].Identifier)
	assert.Equal(t, "Invalid theme.json", check.Results[0].Message)
	assert.Equal(t, validation.SeverityError, check.Results[0].Severity)
}

func TestValidateTheme_MissingPreviewMedia(t *testing.T) {
	tmpDir := t.TempDir()
	resourcesDir := filepath.Join(tmpDir, "Resources")
	err := os.MkdirAll(resourcesDir, 0755)
	assert.NoError(t, err)

	// Create theme.json without previewMedia
	themeJSONPath := filepath.Join(resourcesDir, "theme.json")
	err = os.WriteFile(themeJSONPath, []byte(`{}`), 0644)
	assert.NoError(t, err)

	ext := &mockExtension{
		path:    tmpDir,
		rootDir: tmpDir,
	}

	check := &testCheck{}
	validateTheme(ext, check)

	assert.Len(t, check.Results, 1)
	assert.Equal(t, "Resources/theme.json", check.Results[0].Path)
	assert.Equal(t, "theme.validator", check.Results[0].Identifier)
	assert.Equal(t, "Required field \"previewMedia\" missing in theme.json", check.Results[0].Message)
	assert.Equal(t, validation.SeverityError, check.Results[0].Severity)
}

func TestValidateTheme_EmptyPreviewMedia(t *testing.T) {
	tmpDir := t.TempDir()
	resourcesDir := filepath.Join(tmpDir, "Resources")
	err := os.MkdirAll(resourcesDir, 0755)
	assert.NoError(t, err)

	// Create theme.json with empty previewMedia
	themeJSONPath := filepath.Join(resourcesDir, "theme.json")
	err = os.WriteFile(themeJSONPath, []byte(`{"previewMedia": ""}`), 0644)
	assert.NoError(t, err)

	ext := &mockExtension{
		path:    tmpDir,
		rootDir: tmpDir,
	}

	check := &testCheck{}
	validateTheme(ext, check)

	assert.Len(t, check.Results, 1)
	assert.Equal(t, "Resources/theme.json", check.Results[0].Path)
	assert.Equal(t, "theme.validator", check.Results[0].Identifier)
	assert.Equal(t, "Required field \"previewMedia\" missing in theme.json", check.Results[0].Message)
	assert.Equal(t, validation.SeverityError, check.Results[0].Severity)
}

func TestValidateTheme_PreviewMediaFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	resourcesDir := filepath.Join(tmpDir, "Resources")
	err := os.MkdirAll(resourcesDir, 0755)
	assert.NoError(t, err)

	// Create theme.json with previewMedia pointing to non-existent file
	themeJSONPath := filepath.Join(resourcesDir, "theme.json")
	err = os.WriteFile(themeJSONPath, []byte(`{"previewMedia": "preview.png"}`), 0644)
	assert.NoError(t, err)

	ext := &mockExtension{
		path:    tmpDir,
		rootDir: tmpDir,
	}

	check := &testCheck{}
	validateTheme(ext, check)

	expectedPath := filepath.Join(tmpDir, "src/Resources/preview.png")
	assert.Len(t, check.Results, 1)
	assert.Equal(t, "Resources/theme.json", check.Results[0].Path)
	assert.Equal(t, "theme.validator", check.Results[0].Identifier)
	assert.Contains(t, check.Results[0].Message, "Theme preview image file is expected to be placed at")
	assert.Contains(t, check.Results[0].Message, expectedPath)
	assert.Equal(t, validation.SeverityError, check.Results[0].Severity)
}

func TestValidateTheme_ValidTheme(t *testing.T) {
	tmpDir := t.TempDir()
	resourcesDir := filepath.Join(tmpDir, "Resources")
	err := os.MkdirAll(resourcesDir, 0755)
	assert.NoError(t, err)

	// Create src/Resources directory and preview image
	srcResourcesDir := filepath.Join(tmpDir, "src/Resources")
	err = os.MkdirAll(srcResourcesDir, 0755)
	assert.NoError(t, err)

	previewImagePath := filepath.Join(srcResourcesDir, "preview.png")
	err = createTestImage(previewImagePath)
	assert.NoError(t, err)

	// Create valid theme.json
	themeJSONPath := filepath.Join(resourcesDir, "theme.json")
	err = os.WriteFile(themeJSONPath, []byte(`{"previewMedia": "preview.png"}`), 0644)
	assert.NoError(t, err)

	ext := &mockExtension{
		path:    tmpDir,
		rootDir: tmpDir,
	}

	check := &testCheck{}
	validateTheme(ext, check)

	// Should not add any results when theme is valid
	assert.Empty(t, check.Results)
}

func TestValidateTheme_ValidThemeWithSubdirectory(t *testing.T) {
	tmpDir := t.TempDir()
	resourcesDir := filepath.Join(tmpDir, "Resources")
	err := os.MkdirAll(resourcesDir, 0755)
	assert.NoError(t, err)

	// Create src/Resources/images directory and preview image
	srcResourcesDir := filepath.Join(tmpDir, "src/Resources/images")
	err = os.MkdirAll(srcResourcesDir, 0755)
	assert.NoError(t, err)

	previewImagePath := filepath.Join(srcResourcesDir, "theme-preview.png")
	err = createTestImage(previewImagePath)
	assert.NoError(t, err)

	// Create valid theme.json with subdirectory path
	themeJSONPath := filepath.Join(resourcesDir, "theme.json")
	err = os.WriteFile(themeJSONPath, []byte(`{"previewMedia": "images/theme-preview.png"}`), 0644)
	assert.NoError(t, err)

	ext := &mockExtension{
		path:    tmpDir,
		rootDir: tmpDir,
	}

	check := &testCheck{}
	validateTheme(ext, check)

	// Should not add any results when theme is valid
	assert.Empty(t, check.Results)
}

func TestThemeJSON_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name         string
		jsonContent  string
		expectedJSON themeJSON
		shouldError  bool
	}{
		{
			name:         "valid theme.json",
			jsonContent:  `{"previewMedia": "preview.png"}`,
			expectedJSON: themeJSON{PreviewMedia: "preview.png"},
			shouldError:  false,
		},
		{
			name:         "theme.json with additional fields",
			jsonContent:  `{"previewMedia": "test.jpg", "otherField": "value"}`,
			expectedJSON: themeJSON{PreviewMedia: "test.jpg"},
			shouldError:  false,
		},
		{
			name:        "invalid JSON",
			jsonContent: `{"previewMedia": "test.jpg"`,
			shouldError: true,
		},
		{
			name:         "empty JSON object",
			jsonContent:  `{}`,
			expectedJSON: themeJSON{PreviewMedia: ""},
			shouldError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var theme themeJSON
			err := json.Unmarshal([]byte(tt.jsonContent), &theme)

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedJSON.PreviewMedia, theme.PreviewMedia)
			}
		})
	}
}
