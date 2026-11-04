package verifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTool runs without external tool installations or network access.
func fakeTool(t *testing.T, name, script string) {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte("#!/bin/sh\n"+script), 0o755))
	t.Setenv("PATH", directory)
}

func TestPrettierInvocation(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "write", true: "dry run"}[dryRun], func(t *testing.T) {
			script := "printf 'file.js 1ms\\n'; exit 0\n"
			if dryRun {
				script = "printf 'file.js\\n'; exit 1\n"
			}
			fakeTool(t, "node", "printf '%s\\n' \"$@\" > \"$TOOL_ARGS\"\n"+script)
			root := t.TempDir()
			argsPath := filepath.Join(t.TempDir(), "arguments")
			t.Setenv("TOOL_ARGS", argsPath)
			ignorePath := filepath.Join(root, ".prettierignore")
			require.NoError(t, os.WriteFile(ignorePath, []byte("user ignore rules"), 0o644))
			result := runFormatTool(t, Prettier{}, ToolConfig{RootDir: root, SourceDirectories: []string{root}}, dryRun)
			require.Empty(t, result.Errors)
			arguments, err := os.ReadFile(argsPath)
			require.NoError(t, err)
			assert.Contains(t, string(arguments), "--ignore-path\n.gitignore\n")
			assert.Contains(t, string(arguments), "--ignore-path\n.prettierignore\n")
			if dryRun {
				require.Len(t, result.Findings, 1)
			} else {
				assert.Empty(t, result.Findings)
				assert.Contains(t, result.Note, "formatted")
			}
			ignore, err := os.ReadFile(ignorePath)
			require.NoError(t, err)
			assert.Equal(t, "user ignore rules", string(ignore))
			leftovers, err := filepath.Glob(filepath.Join(root, ".shopware-cli-prettier-ignore-*"))
			require.NoError(t, err)
			assert.Empty(t, leftovers)
		})
	}
}

func TestPHPStanInvocation(t *testing.T) {
	for _, scenario := range []struct {
		name, stdout, stderr, code string
		failed, skipped            bool
		findings                   int
	}{
		{"findings", `{"files":{"file.php":{"messages":[{"line":2,"message":"problem","identifier":"test"}]}},"errors":[]}`, "progress", "1", false, false, 1},
		{"configuration error", `{"files":{},"errors":["invalid configuration"]}`, "", "1", true, false, 0},
		{"no matching files", "No files found to analyse", "", "1", false, true, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fakeTool(t, "php", "printf '%s' \"$TOOL_STDOUT\"\nprintf '%s' \"$TOOL_STDERR\" >&2\nexit \"$TOOL_EXIT\"\n")
			t.Setenv("TOOL_STDOUT", scenario.stdout)
			t.Setenv("TOOL_STDERR", scenario.stderr)
			t.Setenv("TOOL_EXIT", scenario.code)
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte("{}"), 0o644))
			installedDir := filepath.Join(root, "vendor", "composer")
			require.NoError(t, os.MkdirAll(installedDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(installedDir, "installed.json"), []byte(`{"packages":[{"name":"shopware/core","version":"6.7.1.0"}]}`), 0o644))
			result := runCheckTool(t, PhpStan{}, ToolConfig{RootDir: root, SourceDirectories: []string{root}, CheckAgainst: "lowest"})
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.Equal(t, scenario.skipped, result.SkipReason != "")
			assert.Len(t, result.Findings, scenario.findings)
			assert.Equal(t, "Shopware 6.7.1.0", result.Note)
		})
	}
}

func TestPHPCSFixerInvocation(t *testing.T) {
	for _, scenario := range []struct {
		name, output, stderr, code string
		failed                     bool
		findings                   int
	}{
		{"dry run", `{"files":[{"name":"file.php","appliedFixers":["test"]}]}`, "", "8", false, 1},
		{"source syntax error", `{"files":[]}`, "invalid PHP", "4", false, 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			binDir := filepath.Join(root, "php", "vendor", "bin")
			require.NoError(t, os.MkdirAll(binDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(binDir, "php-cs-fixer"), []byte("#!/bin/sh\nprintf '%s' \"$TOOL_STDOUT\"\nprintf '%s' \"$TOOL_STDERR\" >&2\nexit \"$TOOL_EXIT\"\n"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte("{}"), 0o644))
			t.Setenv("TOOL_STDOUT", scenario.output)
			t.Setenv("TOOL_STDERR", scenario.stderr)
			t.Setenv("TOOL_EXIT", scenario.code)
			result := runFormatTool(t, PHPCSFixer{}, ToolConfig{RootDir: root, ToolDirectory: root, SourceDirectories: []string{root}}, true)
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.Len(t, result.Findings, scenario.findings)
		})
	}
}

func TestRectorRestoresComposerFilesOnPreparationFailure(t *testing.T) {
	fakeTool(t, "composer", "printf 'changed' > composer.json\nprintf 'generated' > composer.lock\nprintf 'installation failed' >&2\nexit 1\n")
	root := t.TempDir()
	manifest := []byte(`{"require":{"shopware/core":"*"}}`)
	lock := []byte("original lock")
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), manifest, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.lock"), lock, 0o644))
	result := runFixTool(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: []string{root}, MinShopwareVersion: "6.7.0.0"})
	require.NotEmpty(t, result.Errors)
	assert.Equal(t, "installation failed", result.Errors[0].Stderr)
	actualManifest, err := os.ReadFile(filepath.Join(root, "composer.json"))
	require.NoError(t, err)
	assert.Equal(t, manifest, actualManifest)
	actualLock, err := os.ReadFile(filepath.Join(root, "composer.lock"))
	require.NoError(t, err)
	assert.Equal(t, lock, actualLock)
}

func TestRectorRetainsPreparationAndCleanupFailures(t *testing.T) {
	fakeTool(t, "composer", "/bin/mkdir composer.lock\nprintf 'installation failed' >&2\nexit 1\n")
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.lock"), []byte("original lock"), 0o644))
	result := runFixTool(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: []string{root}})
	require.Len(t, result.Errors, 2)
	assert.Equal(t, "installation failed", result.Errors[0].Stderr)
	var pathError *os.PathError
	require.ErrorAs(t, result.Errors[1], &pathError)
	assert.Equal(t, filepath.Join(root, "composer.lock"), pathError.Path)
}
