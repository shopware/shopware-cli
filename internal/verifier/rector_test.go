package verifier

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/validation"
)

func setupRector(t *testing.T, script string, directories ...string) (string, []string) {
	t.Helper()
	fakeTool(t, "php", script)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte("{}"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(root, "vendor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "rector.php"), []byte("<?php"), 0o644))
	paths := make([]string, 0, len(directories))
	for _, directory := range directories {
		fullPath := filepath.Join(root, directory)
		require.NoError(t, os.MkdirAll(fullPath, 0o755))
		paths = append(paths, fullPath)
	}
	return root, paths
}

func rectorResponder() string {
	return `printf '%s\n' "$@" >> "$TOOL_ARGS"
printf '%s' "$RECTOR_STDERR" >&2
case " $* " in
  *" --dry-run "*) printf '%s' "$RECTOR_VERIFY_OUTPUT"; exit "$RECTOR_VERIFY_EXIT" ;;
  *) printf '%s' "$RECTOR_WRITE_OUTPUT"; exit "$RECTOR_WRITE_EXIT" ;;
esac
`
}

func TestRectorFixReportsAppliedChangesAndVerifies(t *testing.T) {
	root, directories := setupRector(t, rectorResponder(), "src")
	argsPath := filepath.Join(t.TempDir(), "arguments")
	t.Setenv("TOOL_ARGS", argsPath)
	t.Setenv("RECTOR_WRITE_OUTPUT", `{"totals":{"changed_files":1,"errors":0},"file_diffs":[{"file":"src/file.php","diff":"diff","applied_rectors":["TestRector"]}],"changed_files":["src/file.php"]}`)
	t.Setenv("RECTOR_WRITE_EXIT", "0")
	t.Setenv("RECTOR_VERIFY_OUTPUT", `{"totals":{"changed_files":0,"errors":0}}`)
	t.Setenv("RECTOR_VERIFY_EXIT", "0")

	result := runFixTool(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: directories})
	require.Empty(t, result.Errors)
	assert.Empty(t, result.Findings)
	assert.Equal(t, "1 file fixed", result.Note)
	arguments, err := os.ReadFile(argsPath)
	require.NoError(t, err)
	assert.Contains(t, string(arguments), "--output-format\njson")
	assert.Contains(t, string(arguments), "--no-ansi")
	assert.Contains(t, string(arguments), "--no-progress-bar")
	assert.Contains(t, string(arguments), "--dry-run")
	assert.Contains(t, string(arguments), "--clear-cache")
	assert.Contains(t, string(arguments), "--config\n"+filepath.Join(root, "rector.php"))
	assert.Contains(t, string(arguments), "--autoload-file\nvendor/autoload.php")
}

func TestRectorFixReportsVerificationFindings(t *testing.T) {
	root, directories := setupRector(t, rectorResponder(), "src")
	t.Setenv("TOOL_ARGS", filepath.Join(t.TempDir(), "arguments"))
	t.Setenv("RECTOR_WRITE_OUTPUT", `{"totals":{"changed_files":0,"errors":0}}`)
	t.Setenv("RECTOR_WRITE_EXIT", "0")
	t.Setenv("RECTOR_VERIFY_OUTPUT", `{"totals":{"changed_files":1,"errors":0},"file_diffs":[{"file":"src/pending.php","diff":"diff","applied_rectors":["PendingRector"]}],"changed_files":["src/pending.php"]}`)
	t.Setenv("RECTOR_VERIFY_EXIT", "2")

	result := runFixTool(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: directories})
	require.Empty(t, result.Errors)
	assert.ElementsMatch(t, []validation.CheckResult{{
		Path: "src/pending.php", Line: 1,
		Message:  "Rector changes remain after fixing. Review the file and rerun fix.",
		Severity: validation.SeverityError, Identifier: "rector/PendingRector",
	}}, result.Findings)
}

func TestRectorFixReportsErrorsAndRejectsInvalidReports(t *testing.T) {
	for _, scenario := range []struct {
		name, output, exit string
	}{
		{"reported processing error", `{"totals":{"changed_files":0,"errors":1},"errors":[{"message":"broken configuration","caused_by":"TestRector"}]}`, "1"},
		{"internal file error", `{"totals":{"changed_files":0,"errors":1},"errors":[{"message":"internal failure","file":"/tools/vendor/rector/Worker.php","line":3}]}`, "1"},
		{"source-located system error", `{"totals":{"changed_files":0,"errors":1},"errors":[{"message":"System error: rule crashed","file":"src/file.php","line":3}]}`, "1"},
		{"source-located autoload error", `{"totals":{"changed_files":0,"errors":1},"errors":[{"message":"Analyze error: missing class","file":"src/file.php"}]}`, "1"},
		{"rule-caused syntax error", `{"totals":{"changed_files":0,"errors":1},"errors":[{"message":"Syntax error in generated code","file":"src/file.php","caused_by":"TestRector"}]}`, "1"},
		{"malformed json", "not json", "0"},
		{"missing totals", `{}`, "0"},
		{"null report", "null", "0"},
		{"missing errors count", `{"totals":{"changed_files":0}}`, "0"},
		{"inconsistent changed count", `{"totals":{"changed_files":2,"errors":0},"file_diffs":[{"file":"src/file.php","diff":"diff","applied_rectors":["TestRector"]}]}`, "0"},
		{"inconsistent path-only changed count", `{"totals":{"changed_files":2,"errors":0},"changed_files":["src/file.php"]}`, "0"},
		{"empty changed path", `{"totals":{"changed_files":1,"errors":0},"changed_files":[""]}`, "0"},
		{"inconsistent error count", `{"totals":{"changed_files":0,"errors":1}}`, "0"},
		{"incomplete diff", `{"totals":{"changed_files":1,"errors":0},"file_diffs":[{"file":"src/file.php"}]}`, "0"},
		{"empty error", `{"totals":{"changed_files":0,"errors":1},"errors":[{}]}`, "0"},
		{"empty report", "[]", "0"},
		{"fatal process error", "partial", "2"},
		{"unexplained nonzero", `{"totals":{"changed_files":0,"errors":0}}`, "1"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root, directories := setupRector(t, rectorResponder(), "src")
			argsPath := filepath.Join(t.TempDir(), "arguments")
			t.Setenv("TOOL_ARGS", argsPath)
			t.Setenv("RECTOR_WRITE_OUTPUT", scenario.output)
			t.Setenv("RECTOR_WRITE_EXIT", scenario.exit)
			t.Setenv("RECTOR_STDERR", "diagnostic")
			result := runFixTool(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: directories})
			require.NotEmpty(t, result.Errors)
			assert.Empty(t, result.Findings)
			assert.Equal(t, scenario.output, result.Errors[0].Stdout)
			assert.Equal(t, "diagnostic", result.Errors[0].Stderr)
			assert.Equal(t, root, result.Errors[0].Dir)
			arguments, err := os.ReadFile(argsPath)
			require.NoError(t, err)
			assert.NotContains(t, string(arguments), "--dry-run")
		})
	}
}

func TestRectorFixRetainsFindingsFromSuccessfulSibling(t *testing.T) {
	fakeTool(t, "php", `printf '%s\n' "$@" >> "$TOOL_ARGS"
  case " $* " in
    *"/bad "*) printf '{"totals":{"changed_files":0,"errors":1},"errors":[{"message":"broken","file":"bad.php","line":1,"caused_by":"BadRector"}]'; exit 1 ;;
    *" --dry-run "*) printf '{"totals":{"changed_files":1,"errors":0},"file_diffs":[{"file":"good.php","diff":"diff","applied_rectors":["GoodRector"]}],"changed_files":["good.php"]}'; exit 2 ;;
    *) printf '{"totals":{"changed_files":0,"errors":0}}'; exit 0 ;;
esac`)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte("{}"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(root, "vendor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "rector.php"), []byte("<?php"), 0o644))
	good, bad := filepath.Join(root, "good"), filepath.Join(root, "bad")
	require.NoError(t, os.Mkdir(good, 0o755))
	require.NoError(t, os.Mkdir(bad, 0o755))
	argsPath := filepath.Join(t.TempDir(), "arguments")
	t.Setenv("TOOL_ARGS", argsPath)
	result := runFixTool(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: []string{good, bad}})
	require.NotEmpty(t, result.Errors)
	assert.Len(t, result.Findings, 1)
	arguments, err := os.ReadFile(argsPath)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(arguments), "--dry-run"))
}

func TestRectorVerificationOutcomes(t *testing.T) {
	const clean = `{"totals":{"changed_files":0,"errors":0}}`
	const pending = `{"totals":{"changed_files":1,"errors":0},"file_diffs":[{"file":"src/pending.php","diff":"diff","applied_rectors":[]}]}`
	pendingFinding := validation.CheckResult{
		Path: "src/pending.php", Line: 1,
		Message:  "Rector changes remain after fixing. Review the file and rerun fix.",
		Severity: validation.SeverityError, Identifier: "rector/change",
	}
	sourceFinding := validation.CheckResult{
		Path: "src/broken.php", Line: 1, Message: "Syntax error",
		Severity: validation.SeverityError, Identifier: "rector/error",
	}
	for _, scenario := range []struct {
		name, output, exit string
		failed             bool
		findings           []validation.CheckResult
	}{
		{"clean", clean, "0", false, nil},
		{"pending changes", pending, "2", false, []validation.CheckResult{pendingFinding}},
		{"pending changes without diffs", `{"totals":{"changed_files":1,"errors":0},"changed_files":["src/pending.php"]}`, "2", false, []validation.CheckResult{pendingFinding}},
		{"pending changes with zero exit", pending, "0", false, []validation.CheckResult{pendingFinding}},
		{"unexplained failure", clean, "1", true, nil},
		{"unexplained changed exit", clean, "2", true, nil},
		{"invalid report", "not json", "0", true, nil},
		{"source error with partial changes", `{"totals":{"changed_files":1,"errors":1},"file_diffs":[{"file":"src/pending.php","diff":"diff"}],"errors":[{"message":"Syntax error","file":"src/broken.php"}]}`, "1", false, []validation.CheckResult{sourceFinding, pendingFinding}},
		{"global error with partial changes", `{"totals":{"changed_files":1,"errors":1},"file_diffs":[{"file":"src/pending.php","diff":"diff"}],"errors":[{"message":"Worker failed"}]}`, "1", true, []validation.CheckResult{pendingFinding}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root, directories := setupRector(t, rectorResponder(), "src")
			t.Setenv("TOOL_ARGS", filepath.Join(t.TempDir(), "arguments"))
			t.Setenv("RECTOR_WRITE_OUTPUT", clean)
			t.Setenv("RECTOR_WRITE_EXIT", "0")
			t.Setenv("RECTOR_VERIFY_OUTPUT", scenario.output)
			t.Setenv("RECTOR_VERIFY_EXIT", scenario.exit)
			result := runFixTool(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: directories})
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.Equal(t, "no changes", result.Note)
			assert.ElementsMatch(t, scenario.findings, result.Findings)
			if scenario.failed {
				require.NotEmpty(t, result.Errors)
				assert.Equal(t, scenario.output, result.Errors[0].Stdout)
				assert.Contains(t, result.Errors[0].Command, "--dry-run")
			}
		})
	}
}

func TestRectorFixReportsSourceErrors(t *testing.T) {
	root, directories := setupRector(t, rectorResponder(), "src")
	argsPath := filepath.Join(t.TempDir(), "arguments")
	t.Setenv("TOOL_ARGS", argsPath)
	t.Setenv("RECTOR_WRITE_OUTPUT", `{"totals":{"changed_files":0,"errors":1},"errors":[{"file":"src/broken.php","line":8,"message":"Syntax error, unexpected ':'"}]}`)
	t.Setenv("RECTOR_WRITE_EXIT", "1")
	result := runFixTool(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: directories})
	require.Empty(t, result.Errors)
	require.Len(t, result.Findings, 1)
	assert.Equal(t, validation.CheckResult{
		Path: "src/broken.php", Line: 8, Message: "Syntax error, unexpected ':'",
		Severity: validation.SeverityError, Identifier: "rector/error",
	}, result.Findings[0])
	assert.Equal(t, "completed", result.Status())
	arguments, err := os.ReadFile(argsPath)
	require.NoError(t, err)
	assert.NotContains(t, string(arguments), "--dry-run")
}

func TestRectorSourceErrorsThroughSymlinkedRoot(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		mode := "fix"
		if dryRun {
			mode = "dry run"
		}
		for _, location := range []string{"relative", "root alias", "resolved", "outside sources"} {
			t.Run(mode+"/"+location, func(t *testing.T) {
				root, _ := setupRector(t, rectorResponder(), "src", "other")
				require.NoError(t, os.WriteFile(filepath.Join(root, "src", "broken.php"), []byte("<?php invalid"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(root, "other", "broken.php"), []byte("<?php invalid"), 0o644))
				resolved, err := filepath.EvalSymlinks(root)
				require.NoError(t, err)
				alias := filepath.Join(t.TempDir(), "project")
				require.NoError(t, os.Symlink(resolved, alias))

				file := "src/broken.php"
				switch location {
				case "root alias":
					file = filepath.Join(alias, "src", "broken.php")
				case "resolved":
					file = filepath.Join(resolved, "src", "broken.php")
				case "outside sources":
					file = filepath.Join(alias, "other", "broken.php")
				}
				report := `{"totals":{"changed_files":0,"errors":1},"errors":[{"file":"` + file + `","line":8,"message":"Syntax error"}]}`
				t.Setenv("TOOL_ARGS", filepath.Join(t.TempDir(), "arguments"))
				t.Setenv("RECTOR_WRITE_OUTPUT", report)
				t.Setenv("RECTOR_WRITE_EXIT", "1")
				t.Setenv("RECTOR_VERIFY_OUTPUT", report)
				t.Setenv("RECTOR_VERIFY_EXIT", "1")
				result := runFixToolWithDryRun(t, Rector{}, ToolConfig{
					RootDir: alias, SourceDirectories: []string{filepath.Join(resolved, "src")},
				}, dryRun)

				if location == "outside sources" {
					require.Len(t, result.Errors, 1)
					assert.Equal(t, report, result.Errors[0].Stdout)
					assert.Empty(t, result.Findings)
					return
				}
				assert.Empty(t, result.Errors)
				assert.ElementsMatch(t, []validation.CheckResult{{
					Path: "src/broken.php", Line: 8, Message: "Syntax error",
					Severity: validation.SeverityError, Identifier: "rector/error",
				}}, result.Findings)
			})
		}
	}
}

func TestRectorFixWithoutDiffs(t *testing.T) {
	root, directories := setupRector(t, rectorResponder(), "src")
	argsPath := filepath.Join(t.TempDir(), "arguments")
	t.Setenv("TOOL_ARGS", argsPath)
	t.Setenv("RECTOR_WRITE_OUTPUT", `{"totals":{"changed_files":1,"errors":0},"changed_files":["src/file.php"]}`)
	t.Setenv("RECTOR_WRITE_EXIT", "0")
	t.Setenv("RECTOR_VERIFY_OUTPUT", `{"totals":{"changed_files":0,"errors":0}}`)
	t.Setenv("RECTOR_VERIFY_EXIT", "0")
	result := runFixTool(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: directories})
	require.Empty(t, result.Errors)
	assert.Empty(t, result.Findings)
	assert.Equal(t, "1 file fixed", result.Note)
	arguments, err := os.ReadFile(argsPath)
	require.NoError(t, err)
	assert.Contains(t, string(arguments), "--dry-run")
}

func TestRectorSyntaxErrorsOutsideSourcesRemainFailures(t *testing.T) {
	for _, file := range []string{"src-other/broken.php", "src/../other/broken.php", "../outside.php"} {
		t.Run(file, func(t *testing.T) {
			root, directories := setupRector(t, rectorResponder(), "src")
			t.Setenv("TOOL_ARGS", filepath.Join(t.TempDir(), "arguments"))
			t.Setenv("RECTOR_WRITE_OUTPUT", `{"totals":{"changed_files":0,"errors":1},"errors":[{"file":"`+file+`","line":8,"message":"Syntax error"}]}`)
			t.Setenv("RECTOR_WRITE_EXIT", "1")
			t.Setenv("RECTOR_STDERR", "diagnostic")
			result := runFixTool(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: directories})
			require.Len(t, result.Errors, 1)
			assert.Empty(t, result.Findings)
			assert.Contains(t, result.Errors[0].Stdout, file)
			assert.Equal(t, "diagnostic", result.Errors[0].Stderr)
		})
	}
}

func TestRectorRestoresComposerFilesAfterFix(t *testing.T) {
	for _, lockExists := range []bool{false, true} {
		name := "without lock"
		if lockExists {
			name = "with lock"
		}
		t.Run(name, func(t *testing.T) {
			fakeTool(t, "php", `if [ ! -f composer.lock ]; then exit 3; fi
printf '{"totals":{"changed_files":0,"errors":0}}'`)
			composer := filepath.Join(os.Getenv("PATH"), "composer")
			require.NoError(t, os.WriteFile(composer, []byte("#!/bin/sh\nprintf 'changed' > composer.json\nprintf 'generated' > composer.lock\n"), 0o755))
			root := t.TempDir()
			manifest := filepath.Join(root, "composer.json")
			lock := filepath.Join(root, "composer.lock")
			require.NoError(t, os.WriteFile(manifest, []byte("{}"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(root, "rector.php"), []byte("<?php"), 0o644))
			if lockExists {
				require.NoError(t, os.WriteFile(lock, []byte("original"), 0o644))
			}
			result := runFixTool(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: []string{root}})
			assert.Empty(t, result.Errors)
			assert.Empty(t, result.Findings)
			assert.Equal(t, "no changes", result.Note)
			contents, err := os.ReadFile(manifest)
			require.NoError(t, err)
			assert.Equal(t, "{}", string(contents))
			if lockExists {
				contents, err := os.ReadFile(lock)
				require.NoError(t, err)
				assert.Equal(t, "original", string(contents))
			} else {
				_, err := os.Stat(lock)
				assert.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

func TestRectorVerificationPreservesOutsideRootPaths(t *testing.T) {
	root, directories := setupRector(t, rectorResponder(), "src")
	t.Setenv("TOOL_ARGS", filepath.Join(t.TempDir(), "arguments"))
	t.Setenv("RECTOR_WRITE_OUTPUT", `{"totals":{"changed_files":0,"errors":0}}`)
	t.Setenv("RECTOR_WRITE_EXIT", "0")
	t.Setenv("RECTOR_VERIFY_OUTPUT", `{"totals":{"changed_files":1,"errors":0},"changed_files":["../shared/file.php"]}`)
	t.Setenv("RECTOR_VERIFY_EXIT", "2")
	result := runFixTool(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: directories})
	assert.Empty(t, result.Errors)
	assert.ElementsMatch(t, []validation.CheckResult{{
		Path: filepath.Join(filepath.Dir(root), "shared", "file.php"), Line: 1,
		Message:  "Rector changes remain after fixing. Review the file and rerun fix.",
		Severity: validation.SeverityError, Identifier: "rector/change",
	}}, result.Findings)
}

func TestRectorFixDryRunDoesNotWrite(t *testing.T) {
	root, directories := setupRector(t, `dry=false
clear=false
for arg in "$@"; do
    case "$arg" in
        --dry-run) dry=true ;;
        --clear-cache) clear=true ;;
    esac
done
if [ "$dry" = false ]; then printf 'changed' > src/file.php; exit 3; fi
if [ "$clear" = false ]; then printf '{"totals":{"changed_files":0,"errors":0}}'; exit 0; fi
printf '{"totals":{"changed_files":1,"errors":0},"file_diffs":[{"file":"src/file.php","diff":"proposed diff","applied_rectors":["ExampleRector"]}]}'
exit 2`, "src")
	file := filepath.Join(root, "src", "file.php")
	require.NoError(t, os.WriteFile(file, []byte("<?php // unchanged\n"), 0o600))
	config := ToolConfig{RootDir: root, SourceDirectories: directories}
	for range 2 {
		result := runFixToolWithDryRun(t, Rector{}, config, true)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.Note)
		assert.ElementsMatch(t, []validation.CheckResult{{
			Path: "src/file.php", Line: 1,
			Message:  "Rector changes are required. Run fix without dry-run to apply them.",
			Severity: validation.SeverityError, Identifier: "rector/ExampleRector",
		}}, result.Findings)
		contents, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Equal(t, "<?php // unchanged\n", string(contents))
	}
}

func TestRectorFixDryRunOutcomes(t *testing.T) {
	for _, scenario := range []struct {
		name, output, exit string
		failed             bool
		findings           []validation.CheckResult
	}{
		{name: "clean", output: `{"totals":{"changed_files":0,"errors":0}}`, exit: "0"},
		{
			name: "pending changes alongside syntax error", exit: "1",
			output: `{"totals":{"changed_files":1,"errors":1},"changed_files":["src/file.php"],"errors":[{"file":"src/broken.php","line":8,"message":"Syntax error"}]}`,
			findings: []validation.CheckResult{
				{Path: "src/file.php", Line: 1, Severity: validation.SeverityError, Identifier: "rector/change", Message: "Rector changes are required. Run fix without dry-run to apply them."},
				{Path: "src/broken.php", Line: 8, Severity: validation.SeverityError, Identifier: "rector/error", Message: "Syntax error"},
			},
		},
		{name: "fatal report", output: "broken", exit: "1", failed: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root, directories := setupRector(t, rectorResponder(), "src")
			args := filepath.Join(t.TempDir(), "arguments")
			t.Setenv("TOOL_ARGS", args)
			t.Setenv("RECTOR_WRITE_OUTPUT", "Write pass must not run")
			t.Setenv("RECTOR_WRITE_EXIT", "3")
			t.Setenv("RECTOR_VERIFY_OUTPUT", scenario.output)
			t.Setenv("RECTOR_VERIFY_EXIT", scenario.exit)
			t.Setenv("RECTOR_STDERR", "diagnostic")
			result := runFixToolWithDryRun(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: directories}, true)
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.ElementsMatch(t, scenario.findings, result.Findings)
			assert.Empty(t, result.Note)
			if scenario.failed {
				require.Len(t, result.Errors, 1)
				assert.Equal(t, scenario.output, result.Errors[0].Stdout)
				assert.Equal(t, "diagnostic", result.Errors[0].Stderr)
			}
			invocations, err := os.ReadFile(args)
			require.NoError(t, err)
			assert.Equal(t, 1, strings.Count(string(invocations), "--dry-run"))
		})
	}
}
