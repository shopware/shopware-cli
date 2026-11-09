package verifier

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/tracking"
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
  *" --dry-run "*) printf '%s' "$RECTOR_DRY_RUN_OUTPUT"; exit "$RECTOR_DRY_RUN_EXIT" ;;
  *) printf '%s' "$RECTOR_WRITE_OUTPUT"; exit "$RECTOR_WRITE_EXIT" ;;
esac
`
}

func TestRectorFixReportsAppliedChangesWithoutVerification(t *testing.T) {
	root, directories := setupRector(t, rectorResponder(), "src")
	argsPath := filepath.Join(t.TempDir(), "arguments")
	t.Setenv("TOOL_ARGS", argsPath)
	t.Setenv("RECTOR_WRITE_OUTPUT", `{"totals":{"changed_files":1,"errors":0},"file_diffs":[{"file":"src/file.php","diff":"diff","applied_rectors":["TestRector"]}],"changed_files":["src/file.php"]}`)
	t.Setenv("RECTOR_WRITE_EXIT", "0")
	t.Setenv("RECTOR_DRY_RUN_OUTPUT", "unexpected verification")
	t.Setenv("RECTOR_DRY_RUN_EXIT", "3")

	result := runFixTool(t, Rector{}, ToolConfig{IsProject: true, RootDir: root, SourceDirectories: directories})
	require.Empty(t, result.Errors)
	assert.Empty(t, result.Findings)
	assert.Equal(t, "1 file fixed; custom rules from rector.php", result.Note)
	arguments, err := os.ReadFile(argsPath)
	require.NoError(t, err)
	assert.Contains(t, string(arguments), "--output-format\njson")
	assert.Contains(t, string(arguments), "--no-ansi")
	assert.Contains(t, string(arguments), "--no-progress-bar")
	assert.NotContains(t, string(arguments), "--dry-run")
	assert.NotContains(t, string(arguments), "--clear-cache")
	assert.Equal(t, 1, strings.Count(string(arguments), "\nprocess\n"))
	assert.Contains(t, string(arguments), "--config\n"+filepath.Join(root, "rector.php"))
	assert.Contains(t, string(arguments), "--autoload-file\n"+filepath.Join(root, "vendor/autoload.php"))
}

func TestRectorFixCleanInputRunsOnce(t *testing.T) {
	root, directories := setupRector(t, rectorResponder(), "src")
	argsPath := filepath.Join(t.TempDir(), "arguments")
	t.Setenv("TOOL_ARGS", argsPath)
	t.Setenv("RECTOR_WRITE_OUTPUT", `{"totals":{"changed_files":0,"errors":0}}`)
	t.Setenv("RECTOR_WRITE_EXIT", "0")
	t.Setenv("RECTOR_DRY_RUN_OUTPUT", "unexpected verification")
	t.Setenv("RECTOR_DRY_RUN_EXIT", "3")

	result := runFixTool(t, Rector{}, ToolConfig{IsProject: true, RootDir: root, SourceDirectories: directories})
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Findings)
	assert.Equal(t, "no changes; custom rules from rector.php", result.Note)
	arguments, err := os.ReadFile(argsPath)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(arguments), "\nprocess\n"))
	assert.NotContains(t, string(arguments), "--dry-run")
}

func TestRectorFixAppliesChangesAndRetainsManualFindings(t *testing.T) {
	root, directories := setupRector(t, `printf '%s\n' "$@" >> "$TOOL_ARGS"
for arg in "$@"; do
    if [ "$arg" = "--dry-run" ]; then exit 3; fi
done
read -r source < src/fixed.php
changed=0
files=''
if [ "$source" = '<?php // old' ]; then
    printf '<?php // new\n' > src/fixed.php
    changed=1
    files=',"changed_files":["src/fixed.php"]'
fi
printf '{"totals":{"changed_files":%s,"errors":1}%s,"errors":[{"file":"src/broken.php","line":8,"message":"Syntax error"}]}' "$changed" "$files"
exit 1`, "src")
	argsPath := filepath.Join(t.TempDir(), "arguments")
	t.Setenv("TOOL_ARGS", argsPath)
	fixed := filepath.Join(root, "src", "fixed.php")
	broken := filepath.Join(root, "src", "broken.php")
	require.NoError(t, os.WriteFile(fixed, []byte("<?php // old\n"), 0o600))
	require.NoError(t, os.WriteFile(broken, []byte("<?php invalid\n"), 0o600))
	config := ToolConfig{IsProject: true, RootDir: root, SourceDirectories: directories}
	for _, note := range []string{"1 file fixed; custom rules from rector.php", "no changes; custom rules from rector.php"} {
		result := runFixTool(t, Rector{}, config)
		assert.Empty(t, result.Errors)
		assert.Equal(t, note, result.Note)
		assert.ElementsMatch(t, []validation.CheckResult{{
			Path: "src/broken.php", Line: 8, Message: "Syntax error",
			Severity: validation.SeverityError, Identifier: "rector/error",
		}}, result.Findings)
		contents, err := os.ReadFile(fixed)
		require.NoError(t, err)
		assert.Equal(t, "<?php // new\n", string(contents))
		contents, err = os.ReadFile(broken)
		require.NoError(t, err)
		assert.Equal(t, "<?php invalid\n", string(contents))
	}
	arguments, err := os.ReadFile(argsPath)
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(arguments), "\nprocess\n"))
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
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root, directories := setupRector(t, rectorResponder(), "src")
			argsPath := filepath.Join(t.TempDir(), "arguments")
			t.Setenv("TOOL_ARGS", argsPath)
			t.Setenv("RECTOR_WRITE_OUTPUT", scenario.output)
			t.Setenv("RECTOR_WRITE_EXIT", scenario.exit)
			t.Setenv("RECTOR_STDERR", "diagnostic")
			result := runFixTool(t, Rector{}, ToolConfig{IsProject: true, RootDir: root, SourceDirectories: directories})
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
case "$*" in
    *--dry-run*) exit 3 ;;
esac
case " $* " in
    *"/bad "*) printf '{"totals":{"changed_files":0,"errors":1},"errors":[{"message":"broken","file":"bad.php","line":1,"caused_by":"BadRector"}]}'; exit 1 ;;
    *) printf '{"totals":{"changed_files":1,"errors":1},"changed_files":["good/fixed.php"],"errors":[{"message":"Syntax error","file":"good/broken.php","line":2}]}'; exit 1 ;;
esac`)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte("{}"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(root, "vendor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "rector.php"), []byte("<?php"), 0o644))
	good, bad := filepath.Join(root, "good"), filepath.Join(root, "bad")
	require.NoError(t, os.Mkdir(good, 0o755))
	require.NoError(t, os.Mkdir(bad, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(good, "broken.php"), []byte("<?php invalid\n"), 0o644))
	argsPath := filepath.Join(t.TempDir(), "arguments")
	t.Setenv("TOOL_ARGS", argsPath)
	result := runFixTool(t, Rector{}, ToolConfig{IsProject: true, RootDir: root, SourceDirectories: []string{good, bad}})
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Stdout, "BadRector")
	assert.Equal(t, "1 file fixed; custom rules from rector.php", result.Note)
	assert.ElementsMatch(t, []validation.CheckResult{{
		Path: "good/broken.php", Line: 2, Message: "Syntax error",
		Severity: validation.SeverityError, Identifier: "rector/error",
	}}, result.Findings)
	arguments, err := os.ReadFile(argsPath)
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(arguments), "\nprocess\n"))
	assert.NotContains(t, string(arguments), "--dry-run")
}

func TestRectorUnexplainedExitDoesNotClaimNoChanges(t *testing.T) {
	root, directories := setupRector(t, rectorResponder(), "src")
	const output = `{"totals":{"changed_files":0,"errors":0}}`
	t.Setenv("TOOL_ARGS", filepath.Join(t.TempDir(), "arguments"))
	t.Setenv("RECTOR_WRITE_OUTPUT", output)
	t.Setenv("RECTOR_WRITE_EXIT", "1")
	result := runFixTool(t, Rector{}, ToolConfig{IsProject: true, RootDir: root, SourceDirectories: directories})
	require.Len(t, result.Errors, 1)
	assert.Equal(t, output, result.Errors[0].Stdout)
	assert.Empty(t, result.Findings)
	assert.Equal(t, "custom rules from rector.php", result.Note)
}

func TestRectorFixReportsSourceErrors(t *testing.T) {
	root, directories := setupRector(t, rectorResponder(), "src")
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "broken.php"), []byte("<?php :"), 0o644))
	argsPath := filepath.Join(t.TempDir(), "arguments")
	t.Setenv("TOOL_ARGS", argsPath)
	t.Setenv("RECTOR_WRITE_OUTPUT", `{"totals":{"changed_files":0,"errors":1},"errors":[{"file":"src/broken.php","line":8,"message":"Syntax error, unexpected ':'"}]}`)
	t.Setenv("RECTOR_WRITE_EXIT", "1")
	result := runFixTool(t, Rector{}, ToolConfig{IsProject: true, RootDir: root, SourceDirectories: directories})
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
				t.Setenv("RECTOR_DRY_RUN_OUTPUT", report)
				t.Setenv("RECTOR_DRY_RUN_EXIT", "1")
				result := runFixToolWithDryRun(t, Rector{}, ToolConfig{
					IsProject: true, RootDir: alias, SourceDirectories: []string{filepath.Join(resolved, "src")},
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
	t.Setenv("RECTOR_DRY_RUN_OUTPUT", `{"totals":{"changed_files":0,"errors":0}}`)
	t.Setenv("RECTOR_DRY_RUN_EXIT", "0")
	result := runFixTool(t, Rector{}, ToolConfig{IsProject: true, RootDir: root, SourceDirectories: directories})
	require.Empty(t, result.Errors)
	assert.Empty(t, result.Findings)
	assert.Equal(t, "1 file fixed; custom rules from rector.php", result.Note)
	arguments, err := os.ReadFile(argsPath)
	require.NoError(t, err)
	assert.NotContains(t, string(arguments), "--dry-run")
	assert.Equal(t, 1, strings.Count(string(arguments), "\nprocess\n"))
}

func TestRectorSyntaxErrorsOutsideSourcesRemainFailures(t *testing.T) {
	for _, file := range []string{"src-other/broken.php", "src/../other/broken.php", "../outside.php"} {
		t.Run(file, func(t *testing.T) {
			root, directories := setupRector(t, rectorResponder(), "src")
			t.Setenv("TOOL_ARGS", filepath.Join(t.TempDir(), "arguments"))
			t.Setenv("RECTOR_WRITE_OUTPUT", `{"totals":{"changed_files":0,"errors":1},"errors":[{"file":"`+file+`","line":8,"message":"Syntax error"}]}`)
			t.Setenv("RECTOR_WRITE_EXIT", "1")
			t.Setenv("RECTOR_STDERR", "diagnostic")
			result := runFixTool(t, Rector{}, ToolConfig{IsProject: true, RootDir: root, SourceDirectories: directories})
			require.Len(t, result.Errors, 1)
			assert.Empty(t, result.Findings)
			assert.Contains(t, result.Errors[0].Stdout, file)
			assert.Equal(t, "diagnostic", result.Errors[0].Stderr)
		})
	}
}

func TestRectorPreservesComposerFilesAfterFix(t *testing.T) {
	for _, lockExists := range []bool{false, true} {
		name := "without lock"
		if lockExists {
			name = "with lock"
		}
		t.Run(name, func(t *testing.T) {
			fakeTool(t, "php", `if [ ! -f composer.lock ]; then exit 3; fi
printf '{"totals":{"changed_files":0,"errors":0}}'`)
			composer := filepath.Join(os.Getenv("PATH"), "composer")
			require.NoError(t, os.WriteFile(composer, []byte("#!/bin/sh\nprintf 'changed' > composer.json\nprintf 'generated' > composer.lock\n/bin/mkdir -p vendor\n: > vendor/autoload.php\n"), 0o755))
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
			assert.Equal(t, "no changes; custom rules from rector.php", result.Note)
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

func TestRectorDryRunPreservesOutsideRootPaths(t *testing.T) {
	root, directories := setupRector(t, rectorResponder(), "src")
	t.Setenv("TOOL_ARGS", filepath.Join(t.TempDir(), "arguments"))
	t.Setenv("RECTOR_WRITE_OUTPUT", `{"totals":{"changed_files":0,"errors":0}}`)
	t.Setenv("RECTOR_WRITE_EXIT", "0")
	t.Setenv("RECTOR_DRY_RUN_OUTPUT", `{"totals":{"changed_files":1,"errors":0},"changed_files":["../shared/file.php"]}`)
	t.Setenv("RECTOR_DRY_RUN_EXIT", "2")
	result := runFixToolWithDryRun(t, Rector{}, ToolConfig{IsProject: true, RootDir: root, SourceDirectories: directories}, true)
	assert.Empty(t, result.Errors)
	assert.ElementsMatch(t, []validation.CheckResult{{
		Path: filepath.Join(filepath.Dir(root), "shared", "file.php"), Line: 1,
		Message:  "Rector changes are required. Run fix without dry-run to apply them.",
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
	config := ToolConfig{IsProject: true, RootDir: root, SourceDirectories: directories}
	for range 2 {
		result := runFixToolWithDryRun(t, Rector{}, config, true)
		assert.Empty(t, result.Errors)
		assert.Equal(t, "custom rules from rector.php", result.Note)
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
			require.NoError(t, os.WriteFile(filepath.Join(root, "src", "broken.php"), []byte("<?php :"), 0o644))
			args := filepath.Join(t.TempDir(), "arguments")
			t.Setenv("TOOL_ARGS", args)
			t.Setenv("RECTOR_WRITE_OUTPUT", "Write pass must not run")
			t.Setenv("RECTOR_WRITE_EXIT", "3")
			t.Setenv("RECTOR_DRY_RUN_OUTPUT", scenario.output)
			t.Setenv("RECTOR_DRY_RUN_EXIT", scenario.exit)
			t.Setenv("RECTOR_STDERR", "diagnostic")
			result := runFixToolWithDryRun(t, Rector{}, ToolConfig{IsProject: true, RootDir: root, SourceDirectories: directories}, true)
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.ElementsMatch(t, scenario.findings, result.Findings)
			assert.Equal(t, "custom rules from rector.php", result.Note)
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

func TestRectorPreservesComposerFilesOnPreparationFailure(t *testing.T) {
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

func TestRectorDependenciesAndReporting(t *testing.T) {
	for _, scenario := range []struct {
		name                                             string
		localVendor, customConfig, project, fail, dryRun bool
	}{
		{name: "standalone extension"},
		{name: "extension with local vendor and custom config", localVendor: true, customConfig: true},
		{name: "project", localVendor: true, project: true},
		{name: "rector failure", fail: true},
		{name: "extension dry run", localVendor: true, dryRun: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("RECTOR_ROOT", root)
			fakeTools(t, map[string]string{
				"composer": `printf '%s' "$* advisories=$COMPOSER_POLICY_ADVISORIES_BLOCK" > "$RECTOR_ROOT/composer-args"
/bin/mkdir -p vendor/composer
printf '{"packages":[{"name":"shopware/core","version":"6.7.0.0"}]}' > vendor/composer/installed.json
: > vendor/autoload.php
`,
				"php": `printf '%s\n' "$@" > "$RECTOR_ROOT/rector-args"
printf '%s' "$PWD" > "$RECTOR_ROOT/rector-cwd"
printf '{"totals":{"changed_files":0,"errors":0}}'
exit "$RECTOR_EXIT"
`,
			})
			t.Setenv("RECTOR_EXIT", "0")
			if scenario.fail {
				t.Setenv("RECTOR_EXIT", "1")
			}
			manifest := []byte(`{"require":{"shopware/core":"~6.7"}}`)
			lock := []byte("original lockfile")
			require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), manifest, 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(root, "composer.lock"), lock, 0o644))
			if scenario.localVendor {
				require.NoError(t, os.MkdirAll(filepath.Join(root, "vendor/composer"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(root, "vendor/autoload.php"), []byte("original autoloader"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(root, "vendor/composer/installed.json"), []byte(`{"packages":[{"name":"shopware/core","version":"6.7.15.1"}]}`), 0o644))
			}
			if scenario.customConfig {
				require.NoError(t, os.WriteFile(filepath.Join(root, "rector.php"), []byte("<?php"), 0o644))
			}
			tools := t.TempDir()
			cfg := ToolConfig{RootDir: root, ToolDirectory: tools, SourceDirectories: []string{root}, MinShopwareVersion: "6.7.0.0", CheckAgainst: CheckAgainstHighest, IsProject: scenario.project}
			result := runFixToolWithDryRun(t, Rector{}, cfg, scenario.dryRun)
			assert.Equal(t, scenario.fail, len(result.Errors) > 0)
			workDir := filepath.Join(root, "var/cache/shopware-cli/composer-lowest")
			note := "Shopware 6.7.0.0 (lowest); bundled rules for Shopware 6.7"
			expectedConfig := filepath.Join(tools, "php/vendor/frosh/shopware-rector/config/shopware-6.7.0.php")
			if scenario.project {
				workDir = root
				note = "Shopware 6.7.15.1; bundled rules for Shopware 6.7"
				assert.NoFileExists(t, filepath.Join(root, "composer-args"))
			} else {
				composerArgs, err := os.ReadFile(filepath.Join(root, "composer-args"))
				require.NoError(t, err)
				assert.Contains(t, string(composerArgs), "--prefer-lowest")
				assert.Contains(t, string(composerArgs), "advisories=0")
			}
			if scenario.customConfig {
				expectedConfig = filepath.Join(root, "rector.php")
				note = "Shopware 6.7.0.0 (lowest); custom rules from rector.php"
			}
			if !scenario.fail && !scenario.dryRun {
				note = "no changes; " + note
			}
			assert.Equal(t, note, result.Note)
			checkedVersion := "6.7.0.0"
			if scenario.project {
				checkedVersion = "6.7.15.1"
			}
			assert.Equal(t, checkedVersion, tracking.CommandTags()[tracking.TagCheckedShopwareVersion])
			if !scenario.project {
				assert.Equal(t, CheckAgainstLowest, tracking.CommandTags()[tracking.TagCheckAgainst])
			}
			args, err := os.ReadFile(filepath.Join(root, "rector-args"))
			require.NoError(t, err)
			assert.Equal(t, scenario.dryRun, strings.Contains(string(args), "--dry-run\n"))
			assert.Contains(t, string(args), "--config\n"+expectedConfig+"\n")
			assert.Contains(t, string(args), "--autoload-file\n"+filepath.Join(workDir, "vendor/autoload.php")+"\n")
			workingDir, err := os.ReadFile(filepath.Join(root, "rector-cwd"))
			require.NoError(t, err)
			resolvedDir, err := filepath.EvalSymlinks(workDir)
			require.NoError(t, err)
			actualDir, err := filepath.EvalSymlinks(string(workingDir))
			require.NoError(t, err)
			assert.Equal(t, resolvedDir, actualDir)
			for name, expected := range map[string][]byte{"composer.json": manifest, "composer.lock": lock} {
				actual, err := os.ReadFile(filepath.Join(root, name))
				require.NoError(t, err)
				assert.Equal(t, expected, actual)
			}
			if scenario.localVendor {
				autoload, err := os.ReadFile(filepath.Join(root, "vendor/autoload.php"))
				require.NoError(t, err)
				assert.Equal(t, "original autoloader", string(autoload))
			} else {
				assert.NoDirExists(t, filepath.Join(root, "vendor"))
			}
		})
	}
}

func TestRectorExtensionReportsCachedPaths(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		name := "fix"
		if dryRun {
			name = "dry run"
		}
		t.Run(name, func(t *testing.T) {
			fakeTools(t, map[string]string{
				"composer": "/bin/mkdir -p vendor\n: > vendor/autoload.php\n",
				"php": `printf '{"totals":{"changed_files":1,"errors":1},"changed_files":["../../../../src/changed.php"],"errors":[{"file":"../../../../src/broken.php","message":"Syntax error","line":2}]}'
exit 1`,
			})
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte("{}"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(root, "rector.php"), []byte("<?php"), 0o644))
			source := filepath.Join(root, "src")
			require.NoError(t, os.Mkdir(source, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(source, "changed.php"), []byte("<?php"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(source, "broken.php"), []byte("<?php :"), 0o644))
			result := runFixToolWithDryRun(t, Rector{}, ToolConfig{RootDir: root, SourceDirectories: []string{source}}, dryRun)
			require.Empty(t, result.Errors)
			expected := []validation.CheckResult{{
				Path: "src/broken.php", Line: 2, Message: "Syntax error",
				Severity: validation.SeverityError, Identifier: "rector/error",
			}}
			if dryRun {
				expected = append(expected, validation.CheckResult{
					Path: "src/changed.php", Line: 1,
					Message:  "Rector changes are required. Run fix without dry-run to apply them.",
					Severity: validation.SeverityError, Identifier: "rector/change",
				})
			} else {
				assert.Contains(t, result.Note, "1 file fixed")
			}
			assert.ElementsMatch(t, expected, result.Findings)
		})
	}
}
