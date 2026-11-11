package verifier

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
	"github.com/shopware/shopware-cli/internal/validation"
)

func TestHasSCSSFiles(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		files []string
		found bool
	}{
		{"scss file", []string{"styles.scss"}, true},
		{"nested scss", []string{"css/main.scss"}, true},
		{"only css", []string{"styles.css"}, false},
		{"empty directory", nil, false},
		{"ignored node_modules", []string{"node_modules/library.scss"}, false},
		{"ignored vendor", []string{"vendor/library.scss"}, false},
		{"ignored dist", []string{"dist/compiled.scss"}, false},
		{"source alongside dependencies", []string{"node_modules/library.scss", "main.scss"}, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			directory := t.TempDir()
			for _, file := range scenario.files {
				testhelper.WriteFile(t, filepath.Join(directory, file), "")
			}
			found, err := hasSCSSFiles(t.Context(), directory)
			assert.NoError(t, err)
			assert.Equal(t, scenario.found, found)
		})
	}
}

func TestStylelintReportsFindingsAndFailures(t *testing.T) {
	const remainingError = `[{"source":"file.scss","warnings":[{"line":2,"rule":"block-no-empty","severity":"error","text":"Unexpected empty block"}],"autofixed":true}]`
	for _, scenario := range []struct {
		name, stdout, stderr, code, note string
		failed                           bool
		findings                         []validation.CheckResult
	}{
		{
			name:   "clean",
			stdout: `[{"source":"file.scss","warnings":[]}]`,
			code:   "0",
			note:   "no changes",
		},
		{
			name:   "fixed",
			stderr: `[{"source":"file.scss","warnings":[],"autofixed":true}]`,
			code:   "0",
			note:   "1 file fixed",
		},
		{
			name:   "remaining error on stderr",
			stderr: remainingError,
			code:   "2",
			note:   "1 file fixed",
			findings: []validation.CheckResult{{
				Path:       "storefront/file.scss",
				Line:       2,
				Message:    "Unexpected empty block",
				Severity:   validation.SeverityError,
				Identifier: "stylelint/block-no-empty",
			}},
		},
		{
			name:   "remaining error on stdout",
			stdout: remainingError,
			stderr: "tool warning",
			code:   "2",
			note:   "1 file fixed",
			findings: []validation.CheckResult{{
				Path:       "storefront/file.scss",
				Line:       2,
				Message:    "Unexpected empty block",
				Severity:   validation.SeverityError,
				Identifier: "stylelint/block-no-empty",
			}},
		},
		{
			name:   "whitespace stdout",
			stdout: " \n",
			stderr: remainingError,
			code:   "2",
			note:   "1 file fixed",
			findings: []validation.CheckResult{{
				Path:       "storefront/file.scss",
				Line:       2,
				Message:    "Unexpected empty block",
				Severity:   validation.SeverityError,
				Identifier: "stylelint/block-no-empty",
			}},
		},
		{
			name:   "remaining warning",
			stderr: `[{"source":"file.scss","warnings":[{"line":1,"rule":"block-no-empty","severity":"warning","text":"Unexpected empty block"}]}]`,
			code:   "0",
			note:   "no changes",
			findings: []validation.CheckResult{{
				Path:       "storefront/file.scss",
				Line:       1,
				Message:    "Unexpected empty block",
				Severity:   validation.SeverityWarning,
				Identifier: "stylelint/block-no-empty",
			}},
		},
		{
			name:   "deprecation warning",
			stderr: `[{"source":"file.scss","warnings":[],"deprecations":[{"text":"Deprecated rule option","reference":"https://stylelint.io/"}]}]`,
			code:   "0",
			note:   "no changes",
			findings: []validation.CheckResult{{
				Path:       "storefront/file.scss",
				Line:       1,
				Message:    "Deprecated rule option",
				Severity:   validation.SeverityWarning,
				Identifier: "stylelint/diagnostic",
			}},
		},
		{
			name:   "syntax error",
			stderr: `[{"source":"file.scss","warnings":[{"line":3,"rule":"CssSyntaxError","severity":"error","text":"Unclosed block"}]}]`,
			code:   "2",
			note:   "no changes",
			findings: []validation.CheckResult{{
				Path:       "storefront/file.scss",
				Line:       3,
				Message:    "Unclosed block",
				Severity:   validation.SeverityError,
				Identifier: "stylelint/CssSyntaxError",
			}},
		},
		{
			name:   "parse error",
			stderr: `[{"source":"file.scss","warnings":[],"parseErrors":[{"line":3,"text":"Cannot parse source"}]}]`,
			code:   "2",
			note:   "no changes",
			findings: []validation.CheckResult{{
				Path:       "storefront/file.scss",
				Line:       3,
				Message:    "Cannot parse source",
				Severity:   validation.SeverityError,
				Identifier: "stylelint/diagnostic",
			}},
		},
		{
			name:   "invalid options",
			stderr: `[{"source":"file.scss","warnings":[],"invalidOptionWarnings":[{"text":"Invalid option"}]}]`,
			code:   "2",
			failed: true,
		},
		{
			name:   "empty failure",
			stderr: "[]",
			code:   "2",
			failed: true,
		},
		{
			name:   "invalid report",
			stderr: "not json",
			code:   "0",
			failed: true,
		},
		{
			name:   "missing report",
			code:   "0",
			failed: true,
		},
		{
			name:   "null report",
			stderr: "null",
			code:   "0",
			failed: true,
		},
		{
			name:   "empty file result",
			stderr: "[{}]",
			code:   "0",
			failed: true,
		},
		{
			name:   "null file result",
			stderr: "[null]",
			code:   "0",
			failed: true,
		},
		{
			name:   "missing warnings",
			stderr: `[{"source":"file.scss"}]`,
			code:   "0",
			failed: true,
		},
		{
			name:   "missing source",
			stderr: `[{"warnings":[]}]`,
			code:   "0",
			failed: true,
		},
		{
			name:   "fatal error",
			stderr: "fatal tool error",
			code:   "1",
			failed: true,
		},
		{
			name:   "invalid usage",
			stderr: "invalid CLI usage",
			code:   "64",
			failed: true,
		},
		{
			name:   "invalid configuration",
			stderr: "invalid configuration",
			code:   "78",
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
				t.Setenv("TOOL_EXIT", scenario.code)
				root := t.TempDir()
				directory := filepath.Join(root, "storefront")
				require.NoError(t, os.Mkdir(directory, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(directory, "file.scss"), nil, 0o644))
				filePath, err := json.Marshal(filepath.Join(directory, "file.scss"))
				require.NoError(t, err)
				stdout := strings.ReplaceAll(scenario.stdout, `"file.scss"`, string(filePath))
				stderr := strings.ReplaceAll(scenario.stderr, `"file.scss"`, string(filePath))
				t.Setenv("TOOL_STDOUT", stdout)
				t.Setenv("TOOL_STDERR", stderr)
				config := ToolConfig{RootDir: root, StorefrontDirectories: []string{directory}}
				var result validation.ToolReport
				note := ""
				if fix {
					result = runFixTool(t, StyleLint{}, config)
					note = scenario.note
				} else {
					result = runCheckTool(t, StyleLint{}, config)
				}
				assert.Equal(t, scenario.failed, len(result.Errors) > 0)
				assert.ElementsMatch(t, scenario.findings, result.Findings)
				assert.Equal(t, len(scenario.findings) > 0, result.HasFindings)
				assert.Equal(t, note, result.Note)
				if scenario.failed {
					require.NotEmpty(t, result.Errors)
					assert.Equal(t, stdout, result.Errors[0].Stdout)
					assert.Equal(t, stderr, result.Errors[0].Stderr)
					assert.Equal(t, directory, result.Errors[0].Dir)
					assert.NotEmpty(t, result.Errors[0].Command)
				}
			})
		}
	}
}

func TestStylelintCheckReportsProblemsWithoutFixing(t *testing.T) {
	fakeTool(t, "node", `for arg in "$@"; do
    if [ "$arg" = "--fix" ]; then exit 1; fi
done
printf '[{"source":"%s/file.scss","warnings":[{"line":2,"rule":"block-no-empty","severity":"error","text":"Unexpected empty block"}]}]' "$PWD" >&2
exit 2`)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.scss"), nil, 0o644))
	result := runCheckTool(t, StyleLint{}, ToolConfig{RootDir: root, StorefrontDirectories: []string{root}})
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Note)
	assert.ElementsMatch(t, []validation.CheckResult{{
		Path: "file.scss", Line: 2, Message: "Unexpected empty block",
		Severity: validation.SeverityError, Identifier: "stylelint/block-no-empty",
	}}, result.Findings)
}

func TestStylelintFixDryRunReportsFindingsWithoutWriting(t *testing.T) {
	fakeTool(t, "node", `for arg in "$@"; do
    if [ "$arg" = "--fix" ]; then exit 1; fi
done
printf '[{"source":"%s/file.scss","warnings":[{"line":1,"rule":"color-named","severity":"error","text":"Unexpected named color"},{"line":2,"rule":"no-unknown-animations","severity":"error","text":"Unexpected animation"}]}]' "$PWD" >&2
exit 2`)
	root := t.TempDir()
	file := filepath.Join(root, "file.scss")
	require.NoError(t, os.WriteFile(file, []byte("a { color: red; }\n"), 0o600))
	result := runFixToolWithDryRun(t, StyleLint{}, ToolConfig{RootDir: root, AdminDirectories: []string{root}}, true)
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Note)
	assert.True(t, result.HasFindings)
	assert.ElementsMatch(t, []validation.CheckResult{{
		Path: "file.scss", Line: 1, Message: "Unexpected named color",
		Severity: validation.SeverityError, Identifier: "stylelint/color-named",
	}, {
		Path: "file.scss", Line: 2, Message: "Unexpected animation",
		Severity: validation.SeverityError, Identifier: "stylelint/no-unknown-animations",
	}}, result.Findings)
	contents, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "a { color: red; }\n", string(contents))
}

func TestStylelintFixDryRunCleanAndFatalResults(t *testing.T) {
	for _, scenario := range []struct {
		name, output, code string
		failed             bool
	}{
		{name: "clean", output: `[{"source":"%s/file.scss","warnings":[]}]`, code: "0"},
		{name: "fatal", output: "invalid report", code: "0", failed: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fakeTool(t, "node", `for arg in "$@"; do
    if [ "$arg" = "--fix" ]; then exit 1; fi
done
printf '%s' "$TOOL_STDOUT" >&2
exit "$TOOL_EXIT"`)
			root := t.TempDir()
			file := filepath.Join(root, "file.scss")
			require.NoError(t, os.WriteFile(file, []byte("unchanged\n"), 0o600))
			t.Setenv("TOOL_STDOUT", strings.ReplaceAll(scenario.output, "%s", root))
			t.Setenv("TOOL_EXIT", scenario.code)
			result := runFixToolWithDryRun(t, StyleLint{}, ToolConfig{RootDir: root, AdminDirectories: []string{root}}, true)
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.Empty(t, result.Findings)
			assert.Empty(t, result.Note)
			if scenario.failed {
				require.Len(t, result.Errors, 1)
				assert.Empty(t, result.Errors[0].Stdout)
				assert.Equal(t, scenario.output, result.Errors[0].Stderr)
				assert.Equal(t, root, result.Errors[0].Dir)
				assert.NotEmpty(t, result.Errors[0].Command)
			}
			contents, err := os.ReadFile(file)
			require.NoError(t, err)
			assert.Equal(t, "unchanged\n", string(contents))
		})
	}
}

func TestStylelintFixCountsOverlappingDirectoriesOnce(t *testing.T) {
	fakeTool(t, "node", `printf '[{"source":"%s/file.scss","warnings":[],"autofixed":true}]' "$PWD"`)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.scss"), nil, 0o644))
	result := runFixTool(t, StyleLint{}, ToolConfig{RootDir: root, StorefrontDirectories: []string{root, root}})
	assert.Equal(t, "1 file fixed", result.Note)
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Findings)
}

func TestStylelintFixRetainsPartialFindings(t *testing.T) {
	fakeTool(t, "node", `case "$PWD" in
*/bad) printf 'broken'; printf 'diagnostic' >&2; exit 78 ;;
*) printf '[{"source":"%s/file.scss","warnings":[{"rule":"block-no-empty","severity":"error","text":"Unexpected empty block","line":2}],"autofixed":true}]' "$PWD" >&2; exit 2 ;;
esac
`)
	root := t.TempDir()
	good, bad := filepath.Join(root, "good"), filepath.Join(root, "bad")
	for _, directory := range []string{good, bad} {
		require.NoError(t, os.Mkdir(directory, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(directory, "file.scss"), nil, 0o644))
	}
	result := runFixTool(t, StyleLint{}, ToolConfig{RootDir: root, AdminDirectories: []string{good, bad}})
	require.Len(t, result.Errors, 1)
	assert.ElementsMatch(t, []validation.CheckResult{{
		Path: "good/file.scss", Line: 2, Message: "Unexpected empty block",
		Severity: validation.SeverityError, Identifier: "stylelint/block-no-empty",
	}}, result.Findings)
	assert.Equal(t, "broken", result.Errors[0].Stdout)
	assert.Equal(t, "diagnostic", result.Errors[0].Stderr)
	assert.Equal(t, bad, result.Errors[0].Dir)
	assert.Empty(t, result.Note)
}

func TestStylelintRejectsMalformedDiagnostics(t *testing.T) {
	for _, diagnostic := range []string{
		`null`, `{}`, `{"severity":"bogus","text":"problem"}`, `{"severity":"error","text":" "}`,
	} {
		t.Run(diagnostic, func(t *testing.T) {
			fakeTool(t, "node", `printf '[{"source":"%s/file.scss","warnings":[%s]}]' "$PWD" "$DIAGNOSTIC"`)
			t.Setenv("DIAGNOSTIC", diagnostic)
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "file.scss"), nil, 0o644))
			result := runFixTool(t, StyleLint{}, ToolConfig{RootDir: root, AdminDirectories: []string{root}})
			require.Len(t, result.Errors, 1)
			assert.Empty(t, result.Findings)
			assert.Empty(t, result.Note)
		})
	}
}

func TestStylelintMalformedReportExplainsNonzeroExit(t *testing.T) {
	fakeTool(t, "node", `printf '[{}]'; exit 2`)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.scss"), nil, 0o644))
	result := runFixTool(t, StyleLint{}, ToolConfig{RootDir: root, AdminDirectories: []string{root}})
	require.Len(t, result.Errors, 1)
	assert.Empty(t, result.Findings)
	assert.Empty(t, result.Note)
}

func TestStylelintSkipsWithoutSCSS(t *testing.T) {
	fakeTool(t, "node", `printf 'Tool should not run'; exit 1`)
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "node_modules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "node_modules", "file.scss"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.css"), nil, 0o644))
	result := runFixTool(t, StyleLint{}, ToolConfig{RootDir: root, AdminDirectories: []string{root}})
	assert.Equal(t, "skipped", result.Status())
	assert.NotEmpty(t, result.SkipReason)
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Findings)
	assert.Empty(t, result.Note)
}

func TestStylelintDiscoveryFailureIsNotSkipped(t *testing.T) {
	root := t.TempDir()
	result := runFixTool(t, StyleLint{}, ToolConfig{RootDir: root, AdminDirectories: []string{filepath.Join(root, "missing")}})
	require.Len(t, result.Errors, 1)
	assert.ErrorIs(t, result.Errors[0], os.ErrNotExist)
	assert.Equal(t, "failed", result.Status())
	assert.Empty(t, result.SkipReason)
	assert.Empty(t, result.Findings)
	assert.Empty(t, result.Note)
}

func TestStylelintFixCombinesAdministrationAndStorefrontChanges(t *testing.T) {
	fakeTool(t, "node", `printf '[{"source":"%s/file.scss","warnings":[],"autofixed":true}]' "$PWD"`)
	root := t.TempDir()
	admin, storefront := filepath.Join(root, "administration"), filepath.Join(root, "storefront")
	for _, directory := range []string{admin, storefront} {
		require.NoError(t, os.Mkdir(directory, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(directory, "file.scss"), nil, 0o644))
	}
	result := runFixTool(t, StyleLint{}, ToolConfig{
		RootDir: root, AdminDirectories: []string{admin}, StorefrontDirectories: []string{storefront},
	})
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Findings)
	assert.Equal(t, "2 files fixed", result.Note)
}

func TestStylelintRetainsValidFindingsInMalformedReport(t *testing.T) {
	fakeTool(t, "node", `printf '[{"source":"%s/file.scss","autofixed":true,"warnings":[null,{"line":2,"rule":"block-no-empty","severity":"error","text":"Unexpected empty block"}]}]' "$PWD"; exit 2`)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.scss"), nil, 0o644))
	result := runFixTool(t, StyleLint{}, ToolConfig{RootDir: root, AdminDirectories: []string{root}})
	require.Len(t, result.Errors, 1)
	assert.ElementsMatch(t, []validation.CheckResult{{
		Path: "file.scss", Line: 2, Message: "Unexpected empty block",
		Severity: validation.SeverityError, Identifier: "stylelint/block-no-empty",
	}}, result.Findings)
	assert.True(t, result.HasFindings)
	assert.Empty(t, result.Note)
}

func TestStylelintFixAppliesChangesAndBecomesClean(t *testing.T) {
	fakeTool(t, "node", `fix=false
json=false
for arg in "$@"; do
    case "$arg" in
        --fix) fix=true ;;
        --formatter=json) json=true ;;
    esac
done
if [ "$json" = false ]; then printf 'Human-readable output'; exit 0; fi
read -r source < file.scss
if [ "$fix" = true ] && [ "$source" = 'a { color: RED; }' ]; then
    printf 'a { color: red; }\n' > file.scss
    printf '[{"source":"%s/file.scss","warnings":[],"autofixed":true}]' "$PWD" >&2
else
    printf '[{"source":"%s/file.scss","warnings":[]}]' "$PWD" >&2
fi`)
	root := t.TempDir()
	file := filepath.Join(root, "file.scss")
	require.NoError(t, os.WriteFile(file, []byte("a { color: RED; }\n"), 0o600))
	config := ToolConfig{RootDir: root, AdminDirectories: []string{root}}

	first := runFixTool(t, StyleLint{}, config)
	assert.Empty(t, first.Errors)
	assert.Empty(t, first.Findings)
	assert.False(t, first.HasFindings)
	assert.Equal(t, "1 file fixed", first.Note)
	contents, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "a { color: red; }\n", string(contents))

	second := runFixTool(t, StyleLint{}, config)
	assert.Empty(t, second.Errors)
	assert.Empty(t, second.Findings)
	assert.Equal(t, "no changes", second.Note)
}
