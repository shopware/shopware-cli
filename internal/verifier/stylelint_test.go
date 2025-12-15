package verifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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
				err := os.WriteFile(filepath.Join(dir, "styles.scss"), []byte("body { color: red; }"), 0644)
				if err != nil {
					t.Fatal(err)
				}
			},
			expectedResult: true,
		},
		{
			name: "directory with SCSS files in subdirectory",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				subdir := filepath.Join(dir, "css")
				err := os.MkdirAll(subdir, 0755)
				if err != nil {
					t.Fatal(err)
				}
				err = os.WriteFile(filepath.Join(subdir, "main.scss"), []byte("body { color: blue; }"), 0644)
				if err != nil {
					t.Fatal(err)
				}
			},
			expectedResult: true,
		},
		{
			name: "directory without SCSS files",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(dir, "styles.css"), []byte("body { color: red; }"), 0644)
				if err != nil {
					t.Fatal(err)
				}
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
				nodeModules := filepath.Join(dir, "node_modules")
				err := os.MkdirAll(nodeModules, 0755)
				if err != nil {
					t.Fatal(err)
				}
				err = os.WriteFile(filepath.Join(nodeModules, "library.scss"), []byte("body { color: green; }"), 0644)
				if err != nil {
					t.Fatal(err)
				}
			},
			expectedResult: false,
		},
		{
			name: "SCSS files in vendor should be ignored",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				vendor := filepath.Join(dir, "vendor")
				err := os.MkdirAll(vendor, 0755)
				if err != nil {
					t.Fatal(err)
				}
				err = os.WriteFile(filepath.Join(vendor, "library.scss"), []byte("body { color: yellow; }"), 0644)
				if err != nil {
					t.Fatal(err)
				}
			},
			expectedResult: false,
		},
		{
			name: "SCSS files in dist should be ignored",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				dist := filepath.Join(dir, "dist")
				err := os.MkdirAll(dist, 0755)
				if err != nil {
					t.Fatal(err)
				}
				err = os.WriteFile(filepath.Join(dist, "compiled.scss"), []byte("body { color: purple; }"), 0644)
				if err != nil {
					t.Fatal(err)
				}
			},
			expectedResult: false,
		},
		{
			name: "SCSS files outside ignored directories should be found",
			setupFunc: func(t *testing.T, dir string) {
				t.Helper()
				// Create ignored directories with SCSS files
				nodeModules := filepath.Join(dir, "node_modules")
				err := os.MkdirAll(nodeModules, 0755)
				if err != nil {
					t.Fatal(err)
				}
				err = os.WriteFile(filepath.Join(nodeModules, "library.scss"), []byte("body { color: green; }"), 0644)
				if err != nil {
					t.Fatal(err)
				}

				// Create SCSS file in valid location
				err = os.WriteFile(filepath.Join(dir, "main.scss"), []byte("body { color: black; }"), 0644)
				if err != nil {
					t.Fatal(err)
				}
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
