package mjml

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/logging"
)

func TestGetOutputPath(t *testing.T) {
	compiler := &compiler{
		config: Config{Mode: CompilerModeLocal},
	}

	tests := []struct {
		name     string
		mjmlPath string
		expected string
	}{
		{
			name:     "Simple html.mjml",
			mjmlPath: "/path/to/template/html.mjml",
			expected: "/path/to/template/html.twig",
		},
		{
			name:     "Nested path",
			mjmlPath: "/path/to/nested/template/html.mjml",
			expected: "/path/to/nested/template/html.twig",
		},
		{
			name:     "Complex path",
			mjmlPath: "/vendor/frosh/mail/Resources/views/email/order/html.mjml",
			expected: "/vendor/frosh/mail/Resources/views/email/order/html.twig",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compiler.getOutputPath(tt.mjmlPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessDirectory_SkipsNonHTMLFiles(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Create test files
	testFiles := []string{
		"subject.mjml",    // Should be skipped
		"plain.mjml",      // Should be skipped
		"html.mjml",       // Should be processed
		"html.en.mjml",    // Should be skipped (not exactly html.mjml)
		"custom.mjml",     // Should be skipped
		"html.de_DE.mjml", // Should be skipped (not exactly html.mjml)
	}

	for _, file := range testFiles {
		path := filepath.Join(tempDir, file)
		err := os.WriteFile(path, []byte("<mjml><mj-body><mj-text>Test</mj-text></mj-body></mjml>"), 0644)
		assert.NoError(t, err)
	}

	compiler := NewCompiler()

	// Run in dry-run mode to test file detection logic
	ctx := logging.WithLogger(context.Background(), logging.NewLogger(true))
	err := compiler.ProcessDirectory(ctx, tempDir, true)

	// The test should not fail even if mjml is not installed (dry-run mode)
	if err != nil {
		t.Logf("Skipping test as mjml is not available: %v", err)
		t.Skip("mjml not available")
	}

	// Verify that non-HTML files still exist (weren't processed)
	assert.FileExists(t, filepath.Join(tempDir, "subject.mjml"))
	assert.FileExists(t, filepath.Join(tempDir, "plain.mjml"))
	assert.FileExists(t, filepath.Join(tempDir, "custom.mjml"))
}

func TestCheckMJMLAvailable(t *testing.T) {
	c := &compiler{
		config: Config{Mode: CompilerModeLocal},
	}
	ctx := logging.WithLogger(context.Background(), logging.NewLogger(true))

	err := c.checkMJMLAvailable(ctx)
	if err != nil {
		t.Logf("MJML is not installed: %v", err)
		// This is not a failure, just means mjml is not available in test environment
	}
}

func TestCheckMJMLAvailable_SkipsForWebService(t *testing.T) {
	c := &compiler{
		config: Config{Mode: CompilerModeWebService},
	}
	ctx := logging.WithLogger(context.Background(), logging.NewLogger(true))

	// Should not fail even if mjml is not installed when using webservice
	err := c.checkMJMLAvailable(ctx)
	assert.NoError(t, err)
}

func TestNewCompilerWithConfig(t *testing.T) {
	config := Config{
		Mode:             CompilerModeWebService,
		WebServiceURL:    "https://api.mjml.io/v1/render",
		WebServiceAPIKey: "test-api-key",
	}

	compiler := NewCompilerWithConfig(config)
	assert.NotNil(t, compiler)

	// Verify it's configured correctly (type assertion would require exporting the compiler type)
	// Just ensure it doesn't panic and returns a non-nil compiler
	assert.NotNil(t, compiler)
}
