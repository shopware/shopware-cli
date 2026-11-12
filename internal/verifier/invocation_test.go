package verifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/validation"
)

// fakeTool runs without external tool installations or network access.
func fakeTool(t *testing.T, name, script string) {
	t.Helper()
	fakeTools(t, map[string]string{name: script})
}

func fakeTools(t *testing.T, scripts map[string]string) {
	t.Helper()
	directory := t.TempDir()
	for name, script := range scripts {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte("#!/bin/sh\n"+script), 0o755))
	}
	t.Setenv("PATH", directory)
}

func TestPrettierInvocation(t *testing.T) {
	for _, mode := range []string{"write", "dry run", "check"} {
		t.Run(mode, func(t *testing.T) {
			dryRun := mode != "write"
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
			config := ToolConfig{RootDir: root, SourceDirectories: []string{root}}
			var result validation.ToolReport
			if mode == "check" {
				result = runCheckTool(t, Prettier{}, config)
			} else {
				result = runFormatTool(t, Prettier{}, config, dryRun)
			}
			require.Empty(t, result.Errors)
			arguments, err := os.ReadFile(argsPath)
			require.NoError(t, err)
			assert.Contains(t, string(arguments), "--ignore-path\n.gitignore\n")
			assert.Contains(t, string(arguments), "--ignore-path\n.prettierignore\n")
			if dryRun {
				require.Len(t, result.Findings, 1)
				assert.Empty(t, result.ChangedFiles)
				assert.Contains(t, string(arguments), "--list-different\n")
				assert.NotContains(t, string(arguments), "--write")
			} else {
				assert.Empty(t, result.Findings)
				assert.Equal(t, ChangeActionFormatted, result.ChangeAction)
				assert.Equal(t, []string{"file.js"}, result.ChangedFiles)
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

func TestPrettierCheckOutcomes(t *testing.T) {
	for _, scenario := range []struct {
		name, output, code string
		failed             bool
	}{
		{name: "clean", code: "0"},
		{name: "execution failure", output: "diagnostic", code: "2", failed: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fakeTool(t, "node", "printf '%s' \"$TOOL_STDOUT\"\nexit \"$TOOL_EXIT\"\n")
			t.Setenv("TOOL_STDOUT", scenario.output)
			t.Setenv("TOOL_EXIT", scenario.code)
			root := t.TempDir()
			result := runCheckTool(t, Prettier{}, ToolConfig{RootDir: root, SourceDirectories: []string{root}})
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.Empty(t, result.Findings)
			if scenario.failed {
				assert.Equal(t, scenario.output, result.Errors[0].Stdout)
			}
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
			result := runCheckTool(t, PhpStan{}, ToolConfig{RootDir: root, SourceDirectories: []string{root}, IsProject: true})
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.Equal(t, scenario.skipped, result.SkipReason != "")
			assert.Len(t, result.Findings, scenario.findings)
			assert.Equal(t, "Shopware 6.7.1.0", result.Note)
			assert.Equal(t, "6.7.1.0", tracking.CommandTags()[tracking.TagCheckedShopwareVersion])
		})
	}
}

func TestPHPStanExtensionBaseline(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASELINE_ROOT", root)
	fakeTools(t, map[string]string{
		"composer": `/bin/mkdir -p vendor/composer
printf '{"packages":[{"name":"shopware/core","version":"6.6.0.0"}]}' > vendor/composer/installed.json
: > vendor/autoload.php
`,
		"php": `printf '%s\n' "$@" > "$BASELINE_ROOT/php-args"
printf '%s' "$PWD" > "$BASELINE_ROOT/php-cwd"
printf '{"files":{},"errors":[]}'
`,
	})
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"require":{"shopware/core":"^6.6"}}`), 0o644))
	localVendor := filepath.Join(root, "vendor", "composer")
	require.NoError(t, os.MkdirAll(localVendor, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(localVendor, "installed.json"), []byte(`{"packages":[{"name":"shopware/core","version":"6.7.1.0"}]}`), 0o644))
	configFile := filepath.Join(root, "phpstan.neon.dist")
	require.NoError(t, os.WriteFile(configFile, []byte("parameters:\n    level: 5\n"), 0o644))

	result := runCheckTool(t, PhpStan{}, ToolConfig{RootDir: root, SourceDirectories: []string{root}, CheckAgainst: CheckAgainstLowest})

	require.Empty(t, result.Errors)
	assert.Equal(t, "Shopware 6.6.0.0 (lowest)", result.Note)
	assert.Equal(t, "6.6.0.0", tracking.CommandTags()[tracking.TagCheckedShopwareVersion])
	assert.Equal(t, CheckAgainstLowest, tracking.CommandTags()[tracking.TagCheckAgainst])
	workDir := filepath.Join(root, "var/cache/shopware-cli", "composer-lowest")
	arguments, err := os.ReadFile(filepath.Join(root, "php-args"))
	require.NoError(t, err)
	assert.Contains(t, string(arguments), "--autoload-file\n"+filepath.Join(workDir, "vendor/autoload.php")+"\n")
	assert.Contains(t, string(arguments), "--configuration\n"+configFile+"\n")
	workingDir, err := os.ReadFile(filepath.Join(root, "php-cwd"))
	require.NoError(t, err)
	// macOS shells resolve the /var symlink in their working directory.
	expectedDir, err := filepath.EvalSymlinks(workDir)
	require.NoError(t, err)
	assert.Equal(t, expectedDir, string(workingDir))
}

func TestPHPStanSkipsAnalysisAfterFailedDependencyRefresh(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASELINE_ROOT", root)
	fakeTools(t, map[string]string{
		"composer": "exit 1\n",
		"php":      `: > "$BASELINE_ROOT/php-invoked"`,
	})
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{}`), 0o644))
	// An existing cache must not be used when refreshing its dependencies fails.
	cachedVendor := filepath.Join(root, "var/cache/shopware-cli", "composer-lowest", "vendor")
	require.NoError(t, os.MkdirAll(cachedVendor, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cachedVendor, "autoload.php"), nil, 0o644))

	result := runCheckTool(t, PhpStan{}, ToolConfig{RootDir: root, SourceDirectories: []string{root}, CheckAgainst: CheckAgainstLowest})

	require.NotEmpty(t, result.Errors)
	assert.Empty(t, result.Note)
	assert.NoFileExists(t, filepath.Join(root, "php-invoked"))
}

func TestPHPCSFixerInvocation(t *testing.T) {
	for _, scenario := range []struct {
		name, output, stderr, code string
		failed, dryRun             bool
		findings                   int
	}{
		{"write", `{"files":[{"name":"src/file.php","appliedFixers":["test"]}]}`, "", "0", false, false, 0},
		{"clean", `{"files":[]}`, "", "0", false, true, 0},
		{"dry run", `{"files":[{"name":"file.php","appliedFixers":["test"]}]}`, "", "8", false, true, 1},
		{"source syntax error", `{"files":[]}`, "invalid PHP", "4", false, true, 1},
		{"malformed report", `broken`, "diagnostic", "0", true, true, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			binDir := filepath.Join(root, "php", "vendor", "bin")
			require.NoError(t, os.MkdirAll(binDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(binDir, "php-cs-fixer"), []byte("#!/bin/sh\ncase \" $* \" in *' --dry-run '*) actual_dry_run=true ;; *) actual_dry_run=false ;; esac\n[ \"$actual_dry_run\" = \"$TOOL_DRY_RUN\" ] || exit 99\nprintf '%s' \"$TOOL_STDOUT\"\nprintf '%s' \"$TOOL_STDERR\" >&2\nexit \"$TOOL_EXIT\"\n"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte("{}"), 0o644))
			t.Setenv("TOOL_STDOUT", scenario.output)
			t.Setenv("TOOL_STDERR", scenario.stderr)
			t.Setenv("TOOL_EXIT", scenario.code)
			t.Setenv("TOOL_DRY_RUN", "true")
			if !scenario.dryRun {
				t.Setenv("TOOL_DRY_RUN", "false")
			}
			config := ToolConfig{RootDir: root, ToolDirectory: root, SourceDirectories: []string{root}}
			result := runFormatTool(t, PHPCSFixer{}, config, scenario.dryRun)
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.Len(t, result.Findings, scenario.findings)
			if scenario.dryRun {
				assert.Empty(t, result.ChangedFiles)
			} else {
				assert.Equal(t, []string{"src/file.php"}, result.ChangedFiles)
				assert.Equal(t, ChangeActionFormatted, result.ChangeAction)
				return
			}
			checked := runCheckTool(t, PHPCSFixer{}, config)
			assert.Equal(t, scenario.failed, len(checked.Errors) > 0)
			assert.Equal(t, result.Findings, checked.Findings)
			if scenario.failed {
				assert.Equal(t, scenario.output, checked.Errors[0].Stdout)
				assert.Equal(t, scenario.stderr, checked.Errors[0].Stderr)
			}
		})
	}
}

func TestPHPToolsShareLowestDependenciesWithCustomConfigs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TOOL_ROOT", root)
	fakeTools(t, map[string]string{
		"composer": "/bin/mkdir -p vendor\n: > vendor/autoload.php\n",
		"php": `printf '%s\n' "$@" > "$TOOL_ROOT/tool-args"
printf '%s' "$PWD" > "$TOOL_ROOT/tool-cwd"
printf '{"files":{},"totals":{"changed_files":0,"errors":0}}'
`,
	})
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"autoload":{"psr-4":{"Acme\\":"src/"}}}`), 0o644))
	for _, name := range []string{"rector.php", "phpstan.neon"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), nil, 0o644))
	}
	cfg := ToolConfig{RootDir: root, SourceDirectories: []string{root}, CheckAgainst: CheckAgainstLowest}
	workDir := filepath.Join(root, "var/cache/shopware-cli/composer-lowest")
	var previousManifest []byte
	for _, name := range []string{"phpstan", "rector"} {
		t.Run(name, func(t *testing.T) {
			if name == "phpstan" {
				require.Empty(t, runCheckTool(t, PhpStan{}, cfg).Errors)
			} else {
				require.Empty(t, runFixTool(t, Rector{}, cfg).Errors)
			}
			args, err := os.ReadFile(filepath.Join(root, "tool-args"))
			require.NoError(t, err)
			assert.Contains(t, string(args), "--autoload-file\n"+filepath.Join(workDir, "vendor/autoload.php")+"\n")
			cwd, err := os.ReadFile(filepath.Join(root, "tool-cwd"))
			require.NoError(t, err)
			resolvedDir, err := filepath.EvalSymlinks(workDir)
			require.NoError(t, err)
			actualDir, err := filepath.EvalSymlinks(string(cwd))
			require.NoError(t, err)
			assert.Equal(t, resolvedDir, actualDir)
			manifest, err := os.ReadFile(filepath.Join(workDir, "composer.json"))
			require.NoError(t, err)
			assert.Contains(t, string(manifest), filepath.Join(root, "src"))
			if previousManifest != nil {
				assert.JSONEq(t, string(previousManifest), string(manifest))
			}
			previousManifest = manifest
		})
	}
}
