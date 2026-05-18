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

// newArgRecordingMockExec creates a mock npx that writes its full argv to a
// file so tests can assert which CLI flags Compile forwarded.
func newArgRecordingMockExec(t *testing.T, argsFile string) {
	t.Helper()
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
echo "<h1>Hello</h1>"
exit 0
`, argsFile)
	newMockExec(t, script)
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

		output, err := Compile(ctx, tmpFile.Name(), CompileOptions{})
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

		_, err = Compile(ctx, tmpFile.Name(), CompileOptions{})
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

		output, err := Compile(ctx, tmpFile.Name(), CompileOptions{})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if strings.TrimSpace(output) != "<h1>Hello</h1>" {
			t.Errorf("unexpected output: got %q", output)
		}
	})

	t.Run("default options pass no include flags", func(t *testing.T) {
		argsFile := filepath.Join(t.TempDir(), "args.txt")
		newArgRecordingMockExec(t, argsFile)

		tmpFile, err := os.CreateTemp(t.TempDir(), "test.mjml")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}

		if _, err := Compile(ctx, tmpFile.Name(), CompileOptions{}); err != nil {
			t.Fatalf("compile failed: %v", err)
		}

		args := readArgs(t, argsFile)
		if containsArg(args, "--config.allowIncludes=true") {
			t.Errorf("did not expect --config.allowIncludes=true with default options, got %v", args)
		}
		for _, a := range args {
			if strings.HasPrefix(a, "--config.includePath=") {
				t.Errorf("did not expect --config.includePath flag with default options, got %q", a)
			}
		}
	})

	t.Run("allow_includes forwards --config.allowIncludes=true", func(t *testing.T) {
		argsFile := filepath.Join(t.TempDir(), "args.txt")
		newArgRecordingMockExec(t, argsFile)

		tmpFile, err := os.CreateTemp(t.TempDir(), "test.mjml")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}

		if _, err := Compile(ctx, tmpFile.Name(), CompileOptions{AllowIncludes: true}); err != nil {
			t.Fatalf("compile failed: %v", err)
		}

		args := readArgs(t, argsFile)
		if !containsArg(args, "--config.allowIncludes=true") {
			t.Errorf("expected --config.allowIncludes=true in args, got %v", args)
		}
	})

	t.Run("include_paths forwards JSON-encoded array and implies allow_includes", func(t *testing.T) {
		argsFile := filepath.Join(t.TempDir(), "args.txt")
		newArgRecordingMockExec(t, argsFile)

		tmpFile, err := os.CreateTemp(t.TempDir(), "test.mjml")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}

		opts := CompileOptions{IncludePaths: []string{"../_includes", "../_shared"}}
		if _, err := Compile(ctx, tmpFile.Name(), opts); err != nil {
			t.Fatalf("compile failed: %v", err)
		}

		args := readArgs(t, argsFile)
		if !containsArg(args, "--config.allowIncludes=true") {
			t.Errorf("expected include_paths to imply --config.allowIncludes=true, got %v", args)
		}
		want := `--config.includePath=["../_includes","../_shared"]`
		if !containsArg(args, want) {
			t.Errorf("expected %q in args, got %v", want, args)
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

		err := ProcessDirectory(ctx, tmpDir, CompileOptions{})
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

		err := ProcessDirectory(ctx, tmpDir, CompileOptions{})
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

	t.Run("forwards include options to each file", func(t *testing.T) {
		argsFile := filepath.Join(t.TempDir(), "args.txt")
		newArgRecordingMockExec(t, argsFile)

		tmpDir := t.TempDir()
		mailDir := filepath.Join(tmpDir, "mail", "welcome")
		if err := os.MkdirAll(mailDir, 0755); err != nil {
			t.Fatalf("failed to create test dir: %v", err)
		}
		mjmlFile := filepath.Join(mailDir, "html.mjml")
		if err := os.WriteFile(mjmlFile, []byte("<mjml></mjml>"), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		opts := CompileOptions{AllowIncludes: true, IncludePaths: []string{"../_includes"}}
		if err := ProcessDirectory(ctx, tmpDir, opts); err != nil {
			t.Fatalf("ProcessDirectory failed: %v", err)
		}

		args := readArgs(t, argsFile)
		if !containsArg(args, "--config.allowIncludes=true") {
			t.Errorf("expected --config.allowIncludes=true, got %v", args)
		}
		if !containsArg(args, `--config.includePath=["../_includes"]`) {
			t.Errorf("expected JSON-encoded includePath, got %v", args)
		}
	})
}

func readArgs(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func TestNewCompileOptions(t *testing.T) {
	t.Run("returns zero value when nothing is opted in", func(t *testing.T) {
		opts := NewCompileOptions("/abs/search", false, nil)
		if opts.AllowIncludes {
			t.Errorf("expected AllowIncludes=false, got true")
		}
		if len(opts.IncludePaths) != 0 {
			t.Errorf("expected no IncludePaths, got %v", opts.IncludePaths)
		}
	})

	t.Run("allow_includes auto-allowlists the search path root", func(t *testing.T) {
		opts := NewCompileOptions("/abs/search", true, nil)
		if !opts.AllowIncludes {
			t.Errorf("expected AllowIncludes=true")
		}
		want := []string{"/abs/search"}
		if !slicesEqual(opts.IncludePaths, want) {
			t.Errorf("expected IncludePaths=%v, got %v", want, opts.IncludePaths)
		}
	})

	t.Run("extra include paths are appended after the search path root", func(t *testing.T) {
		opts := NewCompileOptions("/abs/search", true, []string{"/abs/other", "/abs/shared"})
		want := []string{"/abs/search", "/abs/other", "/abs/shared"}
		if !slicesEqual(opts.IncludePaths, want) {
			t.Errorf("expected IncludePaths=%v, got %v", want, opts.IncludePaths)
		}
	})

	t.Run("extra include paths alone imply allow_includes", func(t *testing.T) {
		opts := NewCompileOptions("/abs/search", false, []string{"/abs/shared"})
		if !opts.AllowIncludes {
			t.Errorf("expected extras to imply AllowIncludes=true")
		}
		want := []string{"/abs/search", "/abs/shared"}
		if !slicesEqual(opts.IncludePaths, want) {
			t.Errorf("expected IncludePaths=%v, got %v", want, opts.IncludePaths)
		}
	})

	t.Run("empty search path root is skipped", func(t *testing.T) {
		opts := NewCompileOptions("", true, []string{"/abs/shared"})
		want := []string{"/abs/shared"}
		if !slicesEqual(opts.IncludePaths, want) {
			t.Errorf("expected IncludePaths=%v, got %v", want, opts.IncludePaths)
		}
	})
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
