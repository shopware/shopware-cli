package verifier

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPHPCSFixerExpectedExit checks both normal completion and the
// bitmask-based exit statuses reported by PHP-CS-Fixer.
func TestPHPCSFixerExpectedExit(t *testing.T) {
	p := PHPCSFixer{}

	tests := []struct {
		name string
		code int
		want bool
	}{
		{"success", 0, true},
		{"invalid syntax", 4, true},
		{"needs fixing", 8, true},
		{"both findings", 12, true},
		{"general error", 1, false},
		{"configuration error", 16, false},
		{"fixer configuration error", 32, false},
		{"application exception", 64, false},
		{"mixed findings and error", 24, false},
		{"unknown status", 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.code != 0 {
				err = testExitError(t, tt.code)
			}
			if got := p.isExpectedPHPCSFixerExit(err); got != tt.want {
				t.Errorf("isExpectedPHPCSFixerExit(exit %d) = %t, want %t", tt.code, got, tt.want)
			}
		})
	}

	t.Run("non-exit error", func(t *testing.T) {
		if p.isExpectedPHPCSFixerExit(errors.New("failed to start")) {
			t.Fatal("unexpected success for non-exit error")
		}
	})
	t.Run("wrapped exit error", func(t *testing.T) {
		if !p.isExpectedPHPCSFixerExit(errors.Join(errors.New("context"), testExitError(t, 8))) {
			t.Fatal("expected wrapped exit status 8 to be accepted")
		}
	})
}

func TestPHPCSFixerGetConfigPath(t *testing.T) {
	p := PHPCSFixer{}
	root := t.TempDir()
	tools := t.TempDir()
	fallback := filepath.Join(tools, "php", "configs", "php-cs-fixer.dist.php")

	if got := p.getConfigPath(tools, root); got != fallback {
		t.Fatalf("fallback config = %q, want %q", got, fallback)
	}

	projectConfig := filepath.Join(root, ".php-cs-fixer.dist.php")
	if err := os.WriteFile(projectConfig, []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := p.getConfigPath(tools, root); got != projectConfig {
		t.Fatalf("project config = %q, want %q", got, projectConfig)
	}
}

func TestPHPCSFixerFormatComposerManifest(t *testing.T) {
	t.Run("missing manifest skips formatting", func(t *testing.T) {
		result := (PHPCSFixer{}).Format(t.Context(), ToolConfig{RootDir: t.TempDir(), SourceDirectories: []string{"src"}}, false)

		assert.NoError(t, result.Err)
		assert.Empty(t, result.Findings)
	})

	t.Run("other stat errors are returned", func(t *testing.T) {
		// A file as the root makes the manifest lookup fail with a directory error.
		root := filepath.Join(t.TempDir(), "not-a-directory")
		require.NoError(t, os.WriteFile(root, nil, 0o644))

		result := (PHPCSFixer{}).Format(t.Context(), ToolConfig{RootDir: root, SourceDirectories: []string{root}}, false)

		var pathErr *os.PathError
		require.ErrorAs(t, result.Err, &pathErr)
		assert.False(t, errors.Is(result.Err, os.ErrNotExist))
		assert.Empty(t, result.Findings)
	})
}

func TestPHPCSFixerDiagnostics(t *testing.T) {
	invalidSyntax := testExitError(t, 4)
	combinedFindings := testExitError(t, 12)
	formattingDifferences := testExitError(t, 8)
	const diagnostics = "Files that were not fixed due to errors reported during fixing:\nError details"

	tests := []struct {
		name   string
		stderr string
		err    error
		want   string
	}{
		{name: "extracts diagnostics after progress", stderr: "Progress\n\n" + diagnostics + "\n", want: diagnostics},
		{name: "ignores normal stderr and formatting differences", stderr: "Progress\n", err: formattingDifferences},
		{name: "invalid syntax falls back to stderr", stderr: " Syntax error \n", err: invalidSyntax, want: "Syntax error"},
		{name: "invalid syntax falls back to process error", err: invalidSyntax, want: invalidSyntax.Error()},
		{name: "combined findings include invalid syntax", err: combinedFindings, want: combinedFindings.Error()},
		{name: "wrapped invalid syntax error", stderr: "Syntax error", err: errors.Join(errors.New("formatter failed"), invalidSyntax), want: "Syntax error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, phpCsFixerDiagnostics(tt.stderr, tt.err))
		})
	}
}
