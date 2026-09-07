package verifier

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestHasSCSSFiles(t *testing.T) {
	tests := []struct {
		name           string
		setupFunc      func(t *testing.T, dir string)
		expectedResult bool
	}{
		{
			name: "directory with SCSS files",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				testhelper.WriteFile(t, filepath.Join(dir, "styles.scss"), "body { color: red; }")
			},
			expectedResult: true,
		},
		{
			name: "directory with SCSS files in subdirectory",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				testhelper.WriteFile(t, filepath.Join(dir, "css", "main.scss"), "body { color: blue; }")
			},
			expectedResult: true,
		},
		{
			name: "directory without SCSS files",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				testhelper.WriteFile(t, filepath.Join(dir, "styles.css"), "body { color: red; }")
			},
			expectedResult: false,
		},
		{
			name: "empty directory",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				// No files to create
			},
			expectedResult: false,
		},
		{
			name: "SCSS files in node_modules should be ignored",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				testhelper.WriteFile(t, filepath.Join(dir, "node_modules", "library.scss"), "body { color: green; }")
			},
			expectedResult: false,
		},
		{
			name: "SCSS files in vendor should be ignored",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				testhelper.WriteFile(t, filepath.Join(dir, "vendor", "library.scss"), "body { color: yellow; }")
			},
			expectedResult: false,
		},
		{
			name: "SCSS files in dist should be ignored",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				testhelper.WriteFile(t, filepath.Join(dir, "dist", "compiled.scss"), "body { color: purple; }")
			},
			expectedResult: false,
		},
		{
			name: "SCSS files outside ignored directories should be found",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				// Create an ignored directory with SCSS files
				testhelper.WriteFile(t, filepath.Join(dir, "node_modules", "library.scss"), "body { color: green; }")
				// Create SCSS file in valid location
				testhelper.WriteFile(t, filepath.Join(dir, "main.scss"), "body { color: black; }")
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tempDir := t.TempDir()

			// Setup test files
			tt.setupFunc(t, tempDir)

			// Test the function
			result, err := hasSCSSFiles(tempDir)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
