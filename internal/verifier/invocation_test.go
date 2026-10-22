package verifier

import (
	"os"
	"path/filepath"
	"strings"
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

func TestESLintInvocation(t *testing.T) {
	for _, scenario := range []struct {
		name, output, stderr, code string
		failed                     bool
		findings                   int
	}{
		{"findings", `[{"filePath":"file.js","messages":[{"ruleId":"test","severity":2,"message":"problem","line":1}]}]`, "warning on stderr", "1", false, 1},
		{"empty failure", "[]", "", "1", true, 0},
		{"null report", "null", "", "0", true, 0},
		{"invalid output", "not json", "diagnostic", "0", true, 0},
		{"crash", "partial", "crashed", "2", true, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fakeTool(t, "node", "printf '%s' \"$TOOL_STDOUT\"\nprintf '%s' \"$TOOL_STDERR\" >&2\nexit \"$TOOL_EXIT\"\n")
			t.Setenv("TOOL_STDOUT", scenario.output)
			t.Setenv("TOOL_STDERR", scenario.stderr)
			t.Setenv("TOOL_EXIT", scenario.code)
			result := (Eslint{}).Check(t.Context(), ToolConfig{AdminDirectories: []string{t.TempDir()}})
			assert.Equal(t, scenario.failed, result.Err != nil)
			assert.Len(t, result.Findings, scenario.findings)
			assert.Equal(t, scenario.output, result.Stdout)
			assert.Equal(t, scenario.stderr, result.Stderr)
		})
	}
	t.Run("missing executable", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		result := (Eslint{}).Fix(t.Context(), ToolConfig{AdminDirectories: []string{t.TempDir()}})
		require.Error(t, result.Err)
		assert.Empty(t, result.Findings)
	})
}

func TestESLintRetainsPartialFindings(t *testing.T) {
	fakeTool(t, "node", `case "$PWD" in
*/bad) printf 'broken'; printf 'diagnostic' >&2; exit 2 ;;
*) printf '[{"filePath":"file.js","messages":[{"ruleId":"test","severity":2,"message":"problem"}]}]'; exit 1 ;;
esac
`)
	root := t.TempDir()
	good, bad := filepath.Join(root, "good"), filepath.Join(root, "bad")
	require.NoError(t, os.Mkdir(good, 0o755))
	require.NoError(t, os.Mkdir(bad, 0o755))
	result := (Eslint{}).Check(t.Context(), ToolConfig{AdminDirectories: []string{good, bad}})
	require.Error(t, result.Err)
	assert.Len(t, result.Findings, 1)
	assert.Contains(t, result.Stdout, "problem")
	assert.True(t, strings.HasSuffix(result.Stdout, "broken"))
	assert.Equal(t, "diagnostic", result.Stderr)
}

func TestPrettierInvocation(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "write", true: "dry run"}[dryRun], func(t *testing.T) {
			script := "printf 'file.js 1ms\\n'; exit 0\n"
			if dryRun {
				script = "printf 'file.js\\n'; exit 1\n"
			}
			fakeTool(t, "node", script)
			root := t.TempDir()
			ignorePath := filepath.Join(root, ".prettierignore")
			require.NoError(t, os.WriteFile(ignorePath, []byte("user ignore rules"), 0o644))
			result := (Prettier{}).Format(t.Context(), ToolConfig{RootDir: root, SourceDirectories: []string{root}}, dryRun)
			require.NoError(t, result.Err)
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

func TestStylelintFindingsFromStderr(t *testing.T) {
	fakeTool(t, "node", `printf '[{"source":"file.scss","warnings":[{"line":1,"rule":"test","severity":"error","text":"problem"}]}]' >&2
exit 2
`)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.scss"), nil, 0o644))
	result := (StyleLint{}).Fix(t.Context(), ToolConfig{AdminDirectories: []string{root}})
	require.NoError(t, result.Err)
	require.Len(t, result.Findings, 1)
	assert.Equal(t, "stylelint/test", result.Findings[0].Identifier)
	assert.Empty(t, result.Stdout)
	assert.Contains(t, result.Stderr, "problem")
}

func TestPHPStanInvocation(t *testing.T) {
	for _, scenario := range []struct {
		name, stdout, stderr, code string
		failed, skipped            bool
		findings                   int
	}{
		{"findings", `{"files":{"file.php":{"messages":[{"line":2,"message":"problem","identifier":"test"}]}},"errors":[]}`, "progress", "1", false, false, 1},
		{"configuration error", `{"files":{},"errors":["invalid configuration"]}`, "", "1", true, false, 0},
		{"invalid report", "broken", "diagnostic", "0", true, false, 0},
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
			result := (PhpStan{}).Check(t.Context(), ToolConfig{RootDir: root, SourceDirectories: []string{root}, CheckAgainst: "lowest"})
			assert.Equal(t, scenario.failed, result.Err != nil)
			assert.Equal(t, scenario.skipped, result.SkipReason != "")
			assert.Len(t, result.Findings, scenario.findings)
			assert.Equal(t, scenario.stdout, result.Stdout)
			assert.Equal(t, scenario.stderr, result.Stderr)
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
		{"invalid report", "broken", "diagnostic", "0", true, 0},
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
			result := (PHPCSFixer{}).Format(t.Context(), ToolConfig{RootDir: root, ToolDirectory: root, SourceDirectories: []string{root}}, true)
			assert.Equal(t, scenario.failed, result.Err != nil)
			assert.Len(t, result.Findings, scenario.findings)
			assert.Equal(t, scenario.output, result.Stdout)
			assert.Equal(t, scenario.stderr, result.Stderr)
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
	result := (Rector{}).Fix(t.Context(), ToolConfig{RootDir: root, SourceDirectories: []string{root}, MinShopwareVersion: "6.7.0.0"})
	require.Error(t, result.Err)
	assert.Equal(t, "installation failed", result.Stderr)
	actualManifest, err := os.ReadFile(filepath.Join(root, "composer.json"))
	require.NoError(t, err)
	assert.Equal(t, manifest, actualManifest)
	actualLock, err := os.ReadFile(filepath.Join(root, "composer.lock"))
	require.NoError(t, err)
	assert.Equal(t, lock, actualLock)
}
