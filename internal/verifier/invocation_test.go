package verifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/validation"
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
			result := runCheckTool(t, Eslint{}, ToolConfig{AdminDirectories: []string{t.TempDir()}})
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.Len(t, result.Findings, scenario.findings)
			if scenario.failed {
				require.NotEmpty(t, result.Errors)
				assert.Equal(t, scenario.output, result.Errors[0].Stdout)
				assert.Equal(t, scenario.stderr, result.Errors[0].Stderr)
			}
		})
	}
	t.Run("missing executable", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		result := runFixTool(t, Eslint{}, ToolConfig{AdminDirectories: []string{t.TempDir()}})
		require.NotEmpty(t, result.Errors)
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
	result := runCheckTool(t, Eslint{}, ToolConfig{AdminDirectories: []string{good, bad}})
	require.NotEmpty(t, result.Errors)
	assert.Len(t, result.Findings, 1)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "broken", result.Errors[0].Stdout)
	assert.Equal(t, "diagnostic", result.Errors[0].Stderr)
	assert.Equal(t, bad, result.Errors[0].Dir)
	assert.NotEmpty(t, result.Errors[0].Command)
}

func TestToolDirectoriesRunConcurrently(t *testing.T) {
	// Each subprocess waits for its sibling to start. Serial directory execution
	// fails the invocation instead of depending on a wall-clock speed assertion.
	const script = `directory=$PWD
if [ "$TOOL_NAME" = php-cs-fixer ]; then
    for argument do directory=$argument; done
fi
: > "$TOOL_BARRIER/${directory##*/}"
attempt=0
while [ ! -f "$TOOL_BARRIER/first" ] || [ ! -f "$TOOL_BARRIER/second" ]; do
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 250 ]; then
        printf 'sibling directory did not start' >&2
        exit 2
    fi
    /bin/sleep 0.01
done
printf '%s' "$TOOL_STDOUT"
`
	for _, tool := range []Tool{Eslint{}, StyleLint{}, Prettier{}, PHPCSFixer{}} {
		t.Run(tool.Name(), func(t *testing.T) {
			fakeTool(t, "node", script)
			root := t.TempDir()
			t.Setenv("TOOL_NAME", tool.Name())
			t.Setenv("TOOL_BARRIER", t.TempDir())
			t.Setenv("TOOL_STDOUT", map[string]string{
				"eslint": "[]", "stylelint": "[]", "prettier": "file.js 1ms\n",
				"php-cs-fixer": `{"files":[{"name":"file.php","appliedFixers":["test"]}]}`,
			}[tool.Name()])
			first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
			for _, directory := range []string{first, second} {
				require.NoError(t, os.Mkdir(directory, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(directory, "file.scss"), nil, 0o644))
			}
			require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte("{}"), 0o644))
			binDir := filepath.Join(root, "php", "vendor", "bin")
			require.NoError(t, os.MkdirAll(binDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(binDir, "php-cs-fixer"), []byte("#!/bin/sh\n"+script), 0o755))
			config := ToolConfig{RootDir: root, ToolDirectory: root, SourceDirectories: []string{first, second}, AdminDirectories: []string{first, second}}
			var result validation.ToolReport
			if checker, ok := tool.(CheckTool); ok {
				result = runCheckTool(t, checker, config)
			} else {
				result = runFormatTool(t, tool.(FormatTool), config, false)
				assert.Equal(t, "2 files formatted", result.Note)
			}
			require.Empty(t, result.Errors)
		})
	}
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

func TestStylelintFindingsFromStderr(t *testing.T) {
	fakeTool(t, "node", `printf '[{"source":"file.scss","warnings":[{"line":1,"rule":"test","severity":"error","text":"problem"}]}]' >&2
exit 2
`)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.scss"), nil, 0o644))
	result := runCheckTool(t, StyleLint{}, ToolConfig{AdminDirectories: []string{root}})
	require.Empty(t, result.Errors)
	require.Len(t, result.Findings, 1)
	assert.Equal(t, "stylelint/test", result.Findings[0].Identifier)
}

func TestFixersLeaveOutputUnparsed(t *testing.T) {
	for _, tool := range []FixTool{Eslint{}, StyleLint{}} {
		for _, code := range []string{"0", "1"} {
			t.Run(tool.Name()+"/"+code, func(t *testing.T) {
				fakeTool(t, "node", "printf 'unstructured output'; exit \"$TOOL_EXIT\"\n")
				t.Setenv("TOOL_EXIT", code)
				root := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(root, "file.scss"), nil, 0o644))
				result := runFixTool(t, tool, ToolConfig{AdminDirectories: []string{root}})
				assert.Empty(t, result.Findings)
				assert.False(t, result.HasFindings)
				assert.Equal(t, code != "0", len(result.Errors) > 0)
				if code != "0" {
					require.Len(t, result.Errors, 1)
					assert.Equal(t, "unstructured output", result.Errors[0].Stdout)
				}
			})
		}
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
			result := runCheckTool(t, PhpStan{}, ToolConfig{RootDir: root, SourceDirectories: []string{root}, CheckAgainst: "lowest"})
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.Equal(t, scenario.skipped, result.SkipReason != "")
			assert.Len(t, result.Findings, scenario.findings)
			if scenario.failed {
				require.NotEmpty(t, result.Errors)
				assert.Equal(t, scenario.stdout, result.Errors[0].Stdout)
				assert.Equal(t, scenario.stderr, result.Errors[0].Stderr)
			}
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
			result := runFormatTool(t, PHPCSFixer{}, ToolConfig{RootDir: root, ToolDirectory: root, SourceDirectories: []string{root}}, true)
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.Len(t, result.Findings, scenario.findings)
			if scenario.failed {
				require.NotEmpty(t, result.Errors)
				assert.Equal(t, scenario.output, result.Errors[0].Stdout)
				assert.Equal(t, scenario.stderr, result.Errors[0].Stderr)
			}
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
