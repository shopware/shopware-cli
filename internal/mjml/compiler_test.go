package mjml

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetOutputPath(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple case",
			input:    "path/to/html.mjml",
			expected: "path/to/html.twig",
		},
		{
			name:     "Root directory",
			input:    "html.mjml",
			expected: "html.twig",
		},
		{
			name:     "With dots in path",
			input:    "path.with/dots/html.mjml",
			expected: "path.with/dots/html.twig",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := getOutputPath(tc.input)
			if output != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, output)
			}
		})
	}
}

// newMockExec creates a temporary directory with a mock npx executable.
// It adds this directory to the PATH.
func newMockExec(t *testing.T, scriptContent string) {
	t.Helper()

	tmpDir := t.TempDir()

	binDir := filepath.Join(tmpDir, "bin")
	if err := os.Mkdir(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	npxPath := filepath.Join(binDir, "npx")
	err := os.WriteFile(npxPath, []byte(scriptContent), 0755)
	if err != nil {
		t.Fatalf("failed to write mock npx script: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("%s%c%s", binDir, filepath.ListSeparator, originalPath))
}

func TestCompile(t *testing.T) {
	ctx := t.Context()

	t.Run("successful compilation", func(t *testing.T) {
		script := `#!/bin/sh
echo "<h1>Hello</h1>"
exit 0`
		newMockExec(t, script)

		tmpFile, err := os.CreateTemp(t.TempDir(), "test.mjml")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer func() {
			if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
				t.Errorf("failed to remove temp file: %v", removeErr)
			}
		}()

		output, err := Compile(ctx, tmpFile.Name())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if strings.TrimSpace(output) != "<h1>Hello</h1>" {
			t.Errorf("unexpected output: got %q", output)
		}
	})

	t.Run("compilation error", func(t *testing.T) {
		script := `#!/bin/sh
echo "compilation error" >&2
exit 1`
		newMockExec(t, script)

		tmpFile, err := os.CreateTemp(t.TempDir(), "test.mjml")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer func() {
			if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
				t.Errorf("failed to remove temp file: %v", removeErr)
			}
		}()

		_, err = Compile(ctx, tmpFile.Name())
		if err == nil {
			t.Error("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), "mjml compilation failed") {
			t.Errorf("error message should contain 'mjml compilation failed', got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "stderr: compilation error") {
			t.Errorf("error message should contain stderr, got %q", err.Error())
		}
	})

	t.Run("compilation with warnings", func(t *testing.T) {
		script := `#!/bin/sh
echo "a warning" >&2
echo "<h1>Hello</h1>"
exit 0`
		newMockExec(t, script)

		tmpFile, err := os.CreateTemp(t.TempDir(), "test.mjml")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer func() {
			if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
				t.Errorf("failed to remove temp file: %v", removeErr)
			}
		}()

		output, err := Compile(ctx, tmpFile.Name())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if strings.TrimSpace(output) != "<h1>Hello</h1>" {
			t.Errorf("unexpected output: got %q", output)
		}
	})
}

func TestProcessDirectory(t *testing.T) {
	ctx := t.Context()

	t.Run("successful processing", func(t *testing.T) {
		script := `#!/bin/sh
echo "compiled content"
exit 0`
		newMockExec(t, script)

		tmpDir := t.TempDir()

		// Create test file structure
		mailDir1 := filepath.Join(tmpDir, "theme1", "mail", "mail1")
		if err := os.MkdirAll(mailDir1, 0755); err != nil {
			t.Fatalf("failed to create test dir: %v", err)
		}
		mjmlFile1 := filepath.Join(mailDir1, "html.mjml")
		if err := os.WriteFile(mjmlFile1, []byte("<mjml></mjml>"), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		mailDir2 := filepath.Join(tmpDir, "theme2", "mail", "mail2")
		if err := os.MkdirAll(mailDir2, 0755); err != nil {
			t.Fatalf("failed to create test dir: %v", err)
		}
		mjmlFile2 := filepath.Join(mailDir2, "html.mjml")
		if err := os.WriteFile(mjmlFile2, []byte("<mjml></mjml>"), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		// File to be skipped
		skippedMailDir := filepath.Join(tmpDir, "theme2", "mail", "skipped")
		if err := os.MkdirAll(skippedMailDir, 0755); err != nil {
			t.Fatalf("failed to create test dir: %v", err)
		}
		skippedFile := filepath.Join(skippedMailDir, "body.mjml")
		if err := os.WriteFile(skippedFile, []byte("<mjml></mjml>"), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		// File to be ignored
		otherFile := filepath.Join(tmpDir, "other.txt")
		if err := os.WriteFile(otherFile, []byte("hello"), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		err := ProcessDirectory(ctx, tmpDir)
		if err != nil {
			t.Fatalf("ProcessDirectory failed: %v", err)
		}

		// Check results
		// File 1
		if _, err := os.Stat(mjmlFile1); !os.IsNotExist(err) {
			t.Error("original mjml file 1 should have been removed")
		}
		twigFile1 := filepath.Join(mailDir1, "html.twig")
		content, err := os.ReadFile(twigFile1)
		if err != nil {
			t.Errorf("could not read twig file 1: %v", err)
		}
		if strings.TrimSpace(string(content)) != "compiled content" {
			t.Errorf("unexpected content in twig file 1: got %q", string(content))
		}

		// File 2
		if _, err := os.Stat(mjmlFile2); !os.IsNotExist(err) {
			t.Error("original mjml file 2 should have been removed")
		}
		twigFile2 := filepath.Join(mailDir2, "html.twig")
		content, err = os.ReadFile(twigFile2)
		if err != nil {
			t.Errorf("could not read twig file 2: %v", err)
		}
		if strings.TrimSpace(string(content)) != "compiled content" {
			t.Errorf("unexpected content in twig file 2: got %q", string(content))
		}

		// Skipped file
		if _, err := os.Stat(skippedFile); os.IsNotExist(err) {
			t.Error("skipped mjml file should still exist")
		}

		// Ignored file
		if _, err := os.Stat(otherFile); os.IsNotExist(err) {
			t.Error("ignored file should still exist")
		}
	})

	t.Run("processing with compilation errors", func(t *testing.T) {
		script := `#!/bin/sh
echo "error" >&2
exit 1`
		newMockExec(t, script)

		tmpDir := t.TempDir()

		mailDir := filepath.Join(tmpDir, "mail")
		if err := os.MkdirAll(mailDir, 0755); err != nil {
			t.Fatalf("failed to create test dir: %v", err)
		}
		mjmlFile := filepath.Join(mailDir, "html.mjml")
		if err := os.WriteFile(mjmlFile, []byte("<mjml></mjml>"), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		err := ProcessDirectory(ctx, tmpDir)
		if err == nil {
			t.Fatal("expected ProcessDirectory to return an error, but it didn't")
		}
		if !strings.Contains(err.Error(), "compilation completed with 1 errors") {
			t.Errorf("unexpected error message: %q", err.Error())
		}

		// The original file should still exist because processing for it failed
		if _, err := os.Stat(mjmlFile); os.IsNotExist(err) {
			t.Error("original mjml file should still exist after compilation failure")
		}
	})
}
