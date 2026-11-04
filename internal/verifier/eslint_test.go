package verifier

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/validation"
)

func TestESLintReportsFindingsAndFailures(t *testing.T) {
	for _, scenario := range []struct {
		name, output, stderr, code, note string
		failed                           bool
		findings                         []validation.CheckResult
	}{
		{
			name:   "clean",
			output: `[{"filePath":"file.js","messages":[]}]`,
			code:   "0",
			note:   "no changes",
		},
		{
			name:   "fixed",
			output: `[{"filePath":"file.js","messages":[],"output":"const value = 1;"}]`,
			code:   "0",
			note:   "1 file fixed",
		},
		{
			name:   "fixed empty output",
			output: `[{"filePath":"file.js","messages":[],"output":""}]`,
			code:   "0",
			note:   "1 file fixed",
		},
		{
			name:   "remaining error",
			output: `[{"filePath":"file.js","messages":[{"ruleId":"no-undef","severity":2,"message":"Unknown variable","line":2}],"output":"fixed code"}]`,
			stderr: "warning on stderr",
			code:   "1",
			note:   "1 file fixed",
			findings: []validation.CheckResult{{
				Path:       "administration/file.js",
				Line:       2,
				Message:    "Unknown variable",
				Severity:   validation.SeverityError,
				Identifier: "eslint/no-undef",
			}},
		},
		{
			name:   "remaining warning",
			output: `[{"filePath":"file.js","messages":[{"ruleId":"no-unused-vars","severity":1,"message":"Unused variable","line":1}]}]`,
			code:   "0",
			note:   "no changes",
			findings: []validation.CheckResult{{
				Path:       "administration/file.js",
				Line:       1,
				Message:    "Unused variable",
				Severity:   validation.SeverityWarning,
				Identifier: "eslint/no-unused-vars",
			}},
		},
		{
			name:   "syntax error",
			output: `[{"filePath":"file.js","messages":[{"ruleId":null,"severity":2,"fatal":true,"message":"Parsing error","line":3}]}]`,
			code:   "1",
			note:   "no changes",
			findings: []validation.CheckResult{{
				Path:       "administration/file.js",
				Line:       3,
				Message:    "Parsing error",
				Severity:   validation.SeverityError,
				Identifier: "eslint/diagnostic",
			}},
		},
		{
			name:   "empty failure",
			output: "[]",
			code:   "1",
			failed: true,
		},
		{
			name:   "invalid output",
			output: "not json",
			stderr: "diagnostic",
			code:   "0",
			failed: true,
		},
		{
			name:   "missing output",
			code:   "0",
			failed: true,
		},
		{
			name:   "null report",
			output: "null",
			code:   "0",
			failed: true,
		},
		{
			name:   "empty file result",
			output: "[{}]",
			code:   "0",
			failed: true,
		},
		{
			name:   "null file result",
			output: "[null]",
			code:   "0",
			failed: true,
		},
		{
			name:   "missing messages",
			output: `[{"filePath":"file.js"}]`,
			code:   "0",
			failed: true,
		},
		{
			name:   "missing file path",
			output: `[{"messages":[]}]`,
			code:   "0",
			failed: true,
		},
		{
			name:   "null diagnostic",
			output: `[{"filePath":"file.js","messages":[null]}]`,
			code:   "1",
			failed: true,
		},
		{
			name:   "empty diagnostic",
			output: `[{"filePath":"file.js","messages":[{}]}]`,
			code:   "1",
			failed: true,
		},
		{
			name:   "missing severity",
			output: `[{"filePath":"file.js","messages":[{"message":"problem"}]}]`,
			code:   "1",
			failed: true,
		},
		{
			name:   "invalid severity",
			output: `[{"filePath":"file.js","messages":[{"severity":3,"message":"problem"}]}]`,
			code:   "1",
			failed: true,
		},
		{
			name:   "empty diagnostic message",
			output: `[{"filePath":"file.js","messages":[{"severity":2,"message":" "}]}]`,
			code:   "1",
			failed: true,
		},
		{
			name:   "configuration error",
			stderr: "invalid configuration",
			code:   "2",
			failed: true,
		},
	} {
		for _, fix := range []bool{false, true} {
			mode := "check"
			if fix {
				mode = "fix"
			}
			t.Run(mode+"/"+scenario.name, func(t *testing.T) {
				fakeTool(t, "node", `printf '%s' "$TOOL_STDOUT"
printf '%s' "$TOOL_STDERR" >&2
exit "$TOOL_EXIT"`)
				t.Setenv("TOOL_STDERR", scenario.stderr)
				t.Setenv("TOOL_EXIT", scenario.code)
				root := t.TempDir()
				directory := filepath.Join(root, "administration")
				require.NoError(t, os.Mkdir(directory, 0o755))
				filePath, err := json.Marshal(filepath.Join(directory, "file.js"))
				require.NoError(t, err)
				output := strings.ReplaceAll(scenario.output, `"file.js"`, string(filePath))
				t.Setenv("TOOL_STDOUT", output)
				config := ToolConfig{RootDir: root, AdminDirectories: []string{directory}}
				var result validation.ToolReport
				note := ""
				if fix {
					result = runFixTool(t, Eslint{}, config)
					note = scenario.note
				} else {
					result = runCheckTool(t, Eslint{}, config)
				}
				assert.Equal(t, scenario.failed, len(result.Errors) > 0)
				assert.ElementsMatch(t, scenario.findings, result.Findings)
				assert.Equal(t, len(scenario.findings) > 0, result.HasFindings)
				assert.Equal(t, note, result.Note)
				if scenario.failed {
					require.Len(t, result.Errors, 1)
					assert.Equal(t, output, result.Errors[0].Stdout)
					assert.Equal(t, scenario.stderr, result.Errors[0].Stderr)
					assert.Equal(t, directory, result.Errors[0].Dir)
					assert.NotEmpty(t, result.Errors[0].Command)
				}
			})
		}
	}
}

func TestESLintCheckReportsProblemsWithoutFixing(t *testing.T) {
	fakeTool(t, "node", `for arg in "$@"; do
    if [ "$arg" = "--fix" ]; then exit 2; fi
done
printf '[{"filePath":"%s/file.js","messages":[{"ruleId":"no-undef","severity":2,"message":"Unknown variable","line":2}]}]' "$PWD"
exit 1`)
	root := t.TempDir()
	result := runCheckTool(t, Eslint{}, ToolConfig{RootDir: root, AdminDirectories: []string{root}})
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Note)
	assert.True(t, result.HasFindings)
	assert.ElementsMatch(t, []validation.CheckResult{{
		Path: "file.js", Line: 2, Message: "Unknown variable",
		Severity: validation.SeverityError, Identifier: "eslint/no-undef",
	}}, result.Findings)
}

func TestESLintFixCountsOverlappingDirectoriesOnce(t *testing.T) {
	fakeTool(t, "node", `printf '[{"filePath":"%s/file.js","messages":[],"output":"changed"}]' "$PWD"`)
	root := t.TempDir()
	result := runFixTool(t, Eslint{}, ToolConfig{RootDir: root, AdminDirectories: []string{root, root}})
	assert.Equal(t, "1 file fixed", result.Note)
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Findings)
}

func TestESLintFixAppliesChangesAndBecomesClean(t *testing.T) {
	fakeTool(t, "node", `fix=false
json=false
for arg in "$@"; do
    case "$arg" in
        --fix) fix=true ;;
        --format=json) json=true ;;
    esac
done
if [ "$json" = false ]; then printf 'Human-readable output'; exit 0; fi
read -r source < file.js
if [ "$fix" = true ] && [ "$source" = 'let value = 1;' ]; then
    printf 'const value = 1;\n' > file.js
    printf '[{"filePath":"%s/file.js","messages":[],"output":"const value = 1;\\n"}]' "$PWD"
else
    printf '[{"filePath":"%s/file.js","messages":[]}]' "$PWD"
fi`)
	root := t.TempDir()
	file := filepath.Join(root, "file.js")
	require.NoError(t, os.WriteFile(file, []byte("let value = 1;\n"), 0o600))
	config := ToolConfig{RootDir: root, AdminDirectories: []string{root}}

	first := runFixTool(t, Eslint{}, config)
	assert.Empty(t, first.Errors)
	assert.Empty(t, first.Findings)
	assert.False(t, first.HasFindings)
	assert.Equal(t, "1 file fixed", first.Note)
	contents, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "const value = 1;\n", string(contents))

	second := runFixTool(t, Eslint{}, config)
	assert.Empty(t, second.Errors)
	assert.Empty(t, second.Findings)
	assert.Equal(t, "no changes", second.Note)
}

func TestESLintFixCombinesAdministrationAndStorefrontChanges(t *testing.T) {
	fakeTool(t, "node", `printf '[{"filePath":"%s/file.js","messages":[],"output":"changed"}]' "$PWD"`)
	root := t.TempDir()
	admin, storefront := filepath.Join(root, "administration"), filepath.Join(root, "storefront")
	require.NoError(t, os.Mkdir(admin, 0o755))
	require.NoError(t, os.Mkdir(storefront, 0o755))
	result := runFixTool(t, Eslint{}, ToolConfig{
		RootDir: root, AdminDirectories: []string{admin}, StorefrontDirectories: []string{storefront},
	})
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Findings)
	assert.Equal(t, "2 files fixed", result.Note)
}

func TestESLintFixRetainsPartialFindings(t *testing.T) {
	fakeTool(t, "node", `case "$PWD" in
*/bad) printf 'broken'; printf 'diagnostic' >&2; exit 2 ;;
*) printf '[{"filePath":"%s/file.js","messages":[{"ruleId":"no-undef","severity":2,"message":"Unknown variable","line":2}],"output":"fixed code"}]' "$PWD"; exit 1 ;;
esac
`)
	root := t.TempDir()
	good, bad := filepath.Join(root, "good"), filepath.Join(root, "bad")
	require.NoError(t, os.Mkdir(good, 0o755))
	require.NoError(t, os.Mkdir(bad, 0o755))
	result := runFixTool(t, Eslint{}, ToolConfig{RootDir: root, AdminDirectories: []string{good, bad}})
	require.Len(t, result.Errors, 1)
	assert.ElementsMatch(t, []validation.CheckResult{{
		Path: "good/file.js", Line: 2, Message: "Unknown variable",
		Severity: validation.SeverityError, Identifier: "eslint/no-undef",
	}}, result.Findings)
	assert.Equal(t, "broken", result.Errors[0].Stdout)
	assert.Equal(t, "diagnostic", result.Errors[0].Stderr)
	assert.Equal(t, bad, result.Errors[0].Dir)
	assert.Empty(t, result.Note)
}

func TestESLintFixRetainsValidDiagnosticsInMalformedReport(t *testing.T) {
	fakeTool(t, "node", `printf '[{"filePath":"%s/file.js","messages":[null,{"ruleId":"no-unused-vars","severity":1,"message":"Unused variable","line":2}]}]' "$PWD"; exit 1`)
	root := t.TempDir()
	result := runFixTool(t, Eslint{}, ToolConfig{RootDir: root, AdminDirectories: []string{root}})
	require.Len(t, result.Errors, 1)
	assert.ElementsMatch(t, []validation.CheckResult{{
		Path: "file.js", Line: 2, Message: "Unused variable",
		Severity: validation.SeverityWarning, Identifier: "eslint/no-unused-vars",
	}}, result.Findings)
	assert.True(t, result.HasFindings)
	assert.Empty(t, result.Note)
}

func TestESLintFixRetainsValidFilesInMalformedReport(t *testing.T) {
	fakeTool(t, "node", `printf '[null,{"filePath":"%s/file.js","messages":[{"ruleId":"no-undef","severity":2,"message":"Unknown variable","line":2}],"output":"changed"}]' "$PWD"; exit 1`)
	root := t.TempDir()
	result := runFixTool(t, Eslint{}, ToolConfig{RootDir: root, AdminDirectories: []string{root}})
	require.Len(t, result.Errors, 1)
	assert.ElementsMatch(t, []validation.CheckResult{{
		Path: "file.js", Line: 2, Message: "Unknown variable",
		Severity: validation.SeverityError, Identifier: "eslint/no-undef",
	}}, result.Findings)
	assert.True(t, result.HasFindings)
	assert.Empty(t, result.Note)
}

func TestESLintMissingExecutableReportsFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	result := runFixTool(t, Eslint{}, ToolConfig{RootDir: root, AdminDirectories: []string{root}})
	require.Len(t, result.Errors, 1)
	assert.Error(t, result.Errors[0].Err)
	assert.Equal(t, root, result.Errors[0].Dir)
	assert.Empty(t, result.Findings)
	assert.False(t, result.HasFindings)
	assert.Empty(t, result.Note)
}
