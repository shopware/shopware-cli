package verifier

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
)

func TestValidationComposerJSONRequiresSuggestedPackages(t *testing.T) {
	result, err := validationComposerJSON([]byte(`{
		"require": {"shopware/core": "~6.6.0", "psr/log": "^3.0"},
		"suggest": {"psr/log": "already required", "psr/container": "optional"}
	}`), t.TempDir(), "")
	require.NoError(t, err)

	var data struct {
		Require map[string]string `json:"require"`
	}
	require.NoError(t, json.Unmarshal(result, &data))

	assert.Equal(t, map[string]string{
		"shopware/core": "~6.6.0",
		"psr/log":       "^3.0",
		"psr/container": "*",
	}, data.Require)
}

func TestValidationComposerJSONPreservesExtensionAutoload(t *testing.T) {
	root := t.TempDir()
	result, err := validationComposerJSON([]byte(`{
		"autoload": {
			"psr-4": {"Acme\\": ["src/", "/opt/shared/"]},
			"psr-0": {"Legacy_": "legacy/"},
			"files": ["src/functions.php"],
			"classmap": ["classes/"],
			"exclude-from-classmap": ["/classes/excluded/"]
		},
		"autoload-dev": {"psr-4": {"Acme\\Tests\\": "tests/"}}
	}`), root, CheckAgainstLowest)
	require.NoError(t, err)
	var data map[string]map[string]any
	require.NoError(t, json.Unmarshal(result, &data))
	require.Equal(t, map[string]any{
		"psr-4":                 map[string]any{"Acme\\": []any{filepath.Join(root, "src"), "/opt/shared/"}},
		"psr-0":                 map[string]any{"Legacy_": filepath.Join(root, "legacy")},
		"files":                 []any{filepath.Join(root, "src/functions.php")},
		"classmap":              []any{filepath.Join(root, "classes")},
		"exclude-from-classmap": []any{"../../../../classes/excluded/", "../../../../vendor/", "../../../../var/cache/shopware-cli/composer-highest/"},
	}, data["autoload"])
	assert.Equal(t, map[string]any{"exclude-from-classmap": []any{"../../../../vendor/", "../../../../var/cache/shopware-cli/composer-highest/"}, "psr-4": map[string]any{"Acme\\Tests\\": filepath.Join(root, "tests")}}, data["autoload-dev"])

	// Exclusions must still point into the extension if the cache layout changes.
	for _, mode := range []string{CheckAgainstLowest, CheckAgainstHighest} {
		t.Run(mode, func(t *testing.T) {
			workDir := filepath.Join(root, extension.ValidationCacheDir, "composer-"+mode)
			exclusion := data["autoload"]["exclude-from-classmap"].([]any)[0].(string)
			assert.Equal(t, filepath.Join(root, "classes", "excluded"), filepath.Clean(filepath.Join(workDir, exclusion)))
		})
	}
}

func TestValidationComposerJSONAdvisoryBlocking(t *testing.T) {
	for _, mode := range []string{"", CheckAgainstHighest, CheckAgainstLowest} {
		manifest := []byte(`{"config":{"platform":{"php":"8.3"},"audit":{"block-insecure":true,"abandoned":"report"},"policy":{"malware":{"block":true}}}}`)
		result, err := validationComposerJSON(manifest, t.TempDir(), mode)
		require.NoError(t, err)
		var data struct {
			Config struct {
				Platform map[string]string
				Audit    map[string]any
				Policy   map[string]any
			}
		}
		require.NoError(t, json.Unmarshal(result, &data))
		assert.Equal(t, mode != CheckAgainstLowest, data.Config.Audit["block-insecure"])
		assert.Equal(t, "report", data.Config.Audit["abandoned"])
		assert.Equal(t, "8.3", data.Config.Platform["php"])
		assert.Equal(t, map[string]any{"malware": map[string]any{"block": true}}, data.Config.Policy)
	}
}

func TestValidationComposerClassmapIsolation(t *testing.T) {
	for _, tool := range []string{"composer", "php"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is required to check Composer autoloading", tool)
		}
	}
	t.Setenv("COMPOSER_HOME", t.TempDir())
	t.Setenv("COMPOSER_DISABLE_NETWORK", "1")
	for _, mode := range []string{CheckAgainstLowest, CheckAgainstHighest} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			workDir := filepath.Join(root, extension.ValidationCacheDir, "composer-"+mode)
			otherMode := CheckAgainstLowest
			if mode == CheckAgainstLowest {
				otherMode = CheckAgainstHighest
			}
			for file, contents := range map[string]string{
				"src/Extension.php":           "<?php class ExtensionSource {}",
				"tests/Extension.php":         "<?php class ExtensionTest {}",
				"src/Excluded.php":            "<?php class ExcludedSource {}",
				"vendor/local/Dependency.php": "<?php class LocalDependency {}",
				filepath.Join(extension.ValidationCacheDir, "composer-"+otherMode, "vendor/Other.php"):                 "<?php class OtherBaseline {}",
				filepath.Join(extension.ValidationCacheDir, "composer-"+mode, "vendor/acme/dependency/Dependency.php"): "<?php class CachedDependency {}",
				filepath.Join(extension.ValidationCacheDir, "composer-"+mode, "vendor/composer/installed.json"):        `{"packages":[{"name":"acme/dependency","version":"1.0.0","install-path":"../acme/dependency","autoload":{"classmap":["Dependency.php"]}}],"dev":true,"dev-package-names":[]}`,
			} {
				file = filepath.Join(root, file)
				require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
				require.NoError(t, os.WriteFile(file, []byte(contents), 0o644))
			}
			manifest, err := validationComposerJSON([]byte(`{
				"name":"acme/extension",
				"autoload":{"classmap":["."],"exclude-from-classmap":["/src/Excluded.php"]},
				"autoload-dev":{"classmap":["."]},
				"config":{"classmap-authoritative":true}
			}`), root, mode)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(workDir, "composer.json"), manifest, 0o644))
			cmd := exec.CommandContext(t.Context(), "composer", "dump-autoload", "--no-plugins", "--no-scripts", "--no-interaction")
			cmd.Dir = workDir
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			cmd = exec.CommandContext(t.Context(), "php", "-r", `require $argv[1]; foreach (['ExtensionSource', 'ExtensionTest', 'CachedDependency', 'LocalDependency', 'OtherBaseline', 'ExcludedSource'] as $class) { if (class_exists($class)) { echo $class, "\n"; } }`, filepath.Join(workDir, "vendor/autoload.php"))
			output, err = cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			assert.ElementsMatch(t, []string{"ExtensionSource", "ExtensionTest", "CachedDependency"}, strings.Fields(string(output)))
		})
	}
}

func TestValidationComposerJSONResolvesPathRepositories(t *testing.T) {
	rootDir := t.TempDir()
	cases := map[string]string{
		"list":  `[{"type": "path", "url": "packages/*"}, {"type": "path", "url": "/opt/shared"}, {"type": "composer", "url": "https://packages.example.com"}]`,
		"named": `{"local": {"type": "path", "url": "packages/*"}, "shared": {"type": "path", "url": "/opt/shared"}, "store": {"type": "composer", "url": "https://packages.example.com"}, "packagist.org": false}`,
	}

	for name, repositories := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := validationComposerJSON([]byte(`{"repositories": `+repositories+`}`), rootDir, "")
			require.NoError(t, err)

			var data struct {
				Repositories any `json:"repositories"`
			}
			require.NoError(t, json.Unmarshal(result, &data))

			var entries []any
			switch repositories := data.Repositories.(type) {
			case []any:
				entries = repositories
			case map[string]any:
				entries = slices.Collect(maps.Values(repositories))
			}

			var urls []string
			for _, entry := range entries {
				if repository, ok := entry.(map[string]any); ok {
					urls = append(urls, repository["url"].(string))
				}
			}
			assert.ElementsMatch(t, []string{filepath.Join(rootDir, "packages/*"), "/opt/shared", "https://packages.example.com"}, urls)
		})
	}
}

func TestPrepareComposerAutoloadKeepsProjectVendor(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "composer.json"), []byte(`{"require": {"shopware/core": "*"}}`), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "vendor"), 0o755))

	autoloadFile, toolErr := prepareComposerAutoload(t.Context(), ToolConfig{RootDir: rootDir, IsProject: true}, testExecution())
	require.Nil(t, toolErr)

	assert.Equal(t, filepath.Join(rootDir, "vendor", "autoload.php"), autoloadFile)

	entries, err := os.ReadDir(rootDir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"composer.json", "vendor"}, entryNames(entries))
}

func TestPrepareComposerAutoloadRequiresProjectVendor(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "composer.json"), []byte(`{"require": {"shopware/core": "*"}}`), 0o644))

	_, err := prepareComposerAutoload(t.Context(), ToolConfig{RootDir: rootDir, IsProject: true}, testExecution())
	require.NotNil(t, err)

	assert.NoDirExists(t, filepath.Join(rootDir, "vendor"))
	assert.NoDirExists(t, filepath.Join(rootDir, "var"))
}

func TestPrepareComposerAutoloadUpdatesCacheOnEveryRun(t *testing.T) {
	// A fake composer records its arguments and creates the autoload file in
	// the directory it runs in.
	t.Setenv("COMPOSER_POLICY_ADVISORIES_BLOCK", "1")
	runsLog := filepath.Join(t.TempDir(), "runs.log")
	fakeTool(t, "composer", "echo \"$* advisories=$COMPOSER_POLICY_ADVISORIES_BLOCK\" >> '"+runsLog+"'\n/bin/mkdir -p vendor\n: > vendor/autoload.php\n")

	rootDir := t.TempDir()
	composerJSON := []byte(`{"name": "acme/plugin", "require": {"shopware/core": "~6.6.0"}}`)
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "composer.json"), composerJSON, 0o644))

	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "vendor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "vendor", "autoload.php"), []byte("local vendor"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "composer.lock"), []byte("local lockfile"), 0o644))

	modes := []string{"", CheckAgainstLowest, CheckAgainstHighest, CheckAgainstLowest, CheckAgainstHighest}
	for _, checkAgainst := range modes {
		autoloadFile, toolErr := prepareComposerAutoload(t.Context(), ToolConfig{RootDir: rootDir, CheckAgainst: checkAgainst}, testExecution())
		require.Nil(t, toolErr)

		mode := checkAgainst
		if mode == "" {
			mode = CheckAgainstHighest
		}
		workDir := filepath.Join(rootDir, extension.ValidationCacheDir, "composer-"+mode)
		assert.Equal(t, filepath.Join(workDir, "vendor", "autoload.php"), autoloadFile)
		assert.FileExists(t, autoloadFile)

		cached, err := os.ReadFile(filepath.Join(workDir, "composer.json"))
		require.NoError(t, err)
		dependencies, err := validationComposerJSON(composerJSON, rootDir, checkAgainst)
		require.NoError(t, err)
		assert.Equal(t, dependencies, cached)
	}

	runs, err := os.ReadFile(runsLog)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(runs)), "\n")
	require.Len(t, lines, len(modes))
	for i, mode := range modes {
		assert.Equal(t, mode == CheckAgainstLowest, strings.Contains(lines[i], "--prefer-lowest"))
		if mode == CheckAgainstLowest {
			assert.Contains(t, lines[i], "advisories=0")
		} else {
			assert.Contains(t, lines[i], "advisories=1")
		}
	}
	for name, expected := range map[string]string{
		"composer.json":       string(composerJSON),
		"composer.lock":       "local lockfile",
		"vendor/autoload.php": "local vendor",
	} {
		contents, err := os.ReadFile(filepath.Join(rootDir, name))
		require.NoError(t, err)
		assert.Equal(t, expected, string(contents))
	}
}

func TestPrepareComposerAutoloadRunsComposerOneAtATime(t *testing.T) {
	// A fake composer fails when another one is still running.
	running := filepath.Join(t.TempDir(), "running")
	fakeTool(t, "composer", "if [ -e '"+running+"' ]; then exit 1; fi\n: > '"+running+"'\n/bin/sleep 0.3\n/bin/rm '"+running+"'\n/bin/mkdir -p vendor\n: > vendor/autoload.php\n")

	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "composer.json"), []byte(`{"require": {"shopware/core": "*"}}`), 0o644))

	var group sync.WaitGroup
	toolErrs := make([]*ToolError, 2)
	for i := range toolErrs {
		group.Go(func() {
			_, toolErrs[i] = prepareComposerAutoload(t.Context(), ToolConfig{RootDir: rootDir}, testExecution())
		})
	}
	group.Wait()

	for _, toolErr := range toolErrs {
		assert.Nil(t, toolErr)
	}
}

func TestEnsureValidationCacheDirIgnoresItselfInGit(t *testing.T) {
	rootDir := t.TempDir()

	cacheDir, err := ensureValidationCacheDir(rootDir)
	require.NoError(t, err)

	gitignore, err := os.ReadFile(filepath.Join(cacheDir, ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, "*\n", string(gitignore))
}

func testExecution() *ToolExecution {
	return NewToolExecution(func([]validation.CheckResult) bool { return true })
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}
