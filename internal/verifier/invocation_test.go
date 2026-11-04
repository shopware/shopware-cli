package verifier

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestESLintInvocation(t *testing.T) {
	for _, scenario := range []struct {
		name, output, stderr, code string
		failed                     bool
		findings                   int
	}{
		{"findings", `[{"filePath":"file.js","messages":[{"ruleId":"test","severity":2,"message":"problem","line":1}]}]`, "warning on stderr", "1", false, 1},
		{"empty failure", "[]", "", "1", true, 0},
		{"invalid output", "not json", "diagnostic", "0", true, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fakeTool(t, "node", "printf '%s' \"$TOOL_STDOUT\"\nprintf '%s' \"$TOOL_STDERR\" >&2\nexit \"$TOOL_EXIT\"\n")
			t.Setenv("TOOL_STDOUT", scenario.output)
			t.Setenv("TOOL_STDERR", scenario.stderr)
			t.Setenv("TOOL_EXIT", scenario.code)
			result := runCheckTool(t, Eslint{}, ToolConfig{AdminDirectories: []string{t.TempDir()}})
			assert.Equal(t, scenario.failed, len(result.Errors) > 0)
			assert.Len(t, result.Findings, scenario.findings)
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
		for _, code := range []string{"0", "1", "2"} {
			t.Run(tool.Name()+"/"+code, func(t *testing.T) {
				fakeTool(t, "node", "printf 'unstructured output'; printf 'diagnostic' >&2; exit \"$TOOL_EXIT\"\n")
				t.Setenv("TOOL_EXIT", code)
				root := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(root, "file.scss"), nil, 0o644))
				result := runFixTool(t, tool, ToolConfig{AdminDirectories: []string{root}})
				assert.Empty(t, result.Findings)
				assert.Equal(t, code != "0", len(result.Errors) > 0)
				if code != "0" {
					require.Len(t, result.Errors, 1)
					assert.Equal(t, "unstructured output", result.Errors[0].Stdout)
					assert.Equal(t, "diagnostic", result.Errors[0].Stderr)
					var exitErr *exec.ExitError
					require.ErrorAs(t, result.Errors[0], &exitErr)
					expectedCode, err := strconv.Atoi(code)
					require.NoError(t, err)
					assert.Equal(t, expectedCode, exitErr.ExitCode())
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

	result := runCheckTool(t, PhpStan{}, ToolConfig{RootDir: root, SourceDirectories: []string{root}, CheckAgainst: "lowest"})

	require.Empty(t, result.Errors)
	assert.Equal(t, "Shopware 6.6.0.0 (lowest)", result.Note)
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

	result := runCheckTool(t, PhpStan{}, ToolConfig{RootDir: root, SourceDirectories: []string{root}, CheckAgainst: "lowest"})

	require.NotEmpty(t, result.Errors)
	assert.Empty(t, result.Note)
	assert.NoFileExists(t, filepath.Join(root, "php-invoked"))
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

func TestPHPToolsShareLowestDependenciesWithCustomConfigs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TOOL_ROOT", root)
	fakeTools(t, map[string]string{
		"composer": "/bin/mkdir -p vendor\n: > vendor/autoload.php\n",
		"php": `printf '%s\n' "$@" > "$TOOL_ROOT/tool-args"
printf '%s' "$PWD" > "$TOOL_ROOT/tool-cwd"
printf '{"files":{}}'
`,
	})
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"autoload":{"psr-4":{"Acme\\":"src/"}}}`), 0o644))
	for _, name := range []string{"rector.php", "phpstan.neon"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), nil, 0o644))
	}
	cfg := ToolConfig{RootDir: root, SourceDirectories: []string{root}, CheckAgainst: "lowest"}
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

func TestRectorDependenciesAndReporting(t *testing.T) {
	for _, scenario := range []struct {
		name                                     string
		localVendor, customConfig, project, fail bool
	}{
		{name: "standalone extension"},
		{name: "extension with local vendor and custom config", localVendor: true, customConfig: true},
		{name: "project", localVendor: true, project: true},
		{name: "rector failure", fail: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("RECTOR_ROOT", root)
			fakeTool(t, "composer", `printf '%s' "$* advisories=$COMPOSER_POLICY_ADVISORIES_BLOCK" > "$RECTOR_ROOT/composer-args"
/bin/mkdir -p vendor/composer
printf '{"packages":[{"name":"shopware/core","version":"6.7.0.0"}]}' > vendor/composer/installed.json
: > vendor/autoload.php
`)
			composerPath := os.Getenv("PATH")
			fakeTool(t, "php", `printf '%s\n' "$@" > "$RECTOR_ROOT/rector-args"
printf '%s' "$PWD" > "$RECTOR_ROOT/rector-cwd"
exit "$RECTOR_EXIT"
`)
			t.Setenv("PATH", os.Getenv("PATH")+string(os.PathListSeparator)+composerPath)
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
			cfg := ToolConfig{RootDir: root, ToolDirectory: tools, SourceDirectories: []string{root}, MinShopwareVersion: "6.7.0.0", CheckAgainst: "highest", IsProject: scenario.project}
			result := runFixTool(t, Rector{}, cfg)
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
			assert.Equal(t, note, result.Note)
			args, err := os.ReadFile(filepath.Join(root, "rector-args"))
			require.NoError(t, err)
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
