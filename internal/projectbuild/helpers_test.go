package projectbuild

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestCleanupTcpdfKeepsOnlyCourierAndHelvetica(t *testing.T) {
	root := t.TempDir()
	fonts := filepath.Join(root, "vendor", "tecnickcom", "tcpdf", "fonts")
	for _, name := range []string{"helvetica.php", "courier_bold.php", "times.php", "foo.z", ".z"} {
		testhelper.WriteFile(t, filepath.Join(fonts, name), "x")
	}
	require.NoError(t, cleanupTcpdf(t.Context(), root))
	assert.FileExists(t, filepath.Join(fonts, "helvetica.php"))
	assert.FileExists(t, filepath.Join(fonts, "courier_bold.php"))
	assert.NoFileExists(t, filepath.Join(fonts, "times.php"))
	assert.NoFileExists(t, filepath.Join(fonts, "foo.z"))
	assert.NoFileExists(t, filepath.Join(fonts, ".z"))
	require.NoError(t, cleanupTcpdf(t.Context(), t.TempDir()))
}

func TestExecuteCIHooksRunsInRootWithBuildEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hooks run through sh -c")
	}
	root := t.TempDir()
	env := buildEnvironment(func(string) string { return "" })
	require.NoError(t, executeCIHooks(t.Context(), "test-hooks", []string{`printf '%s\n' "$PROJECT_ROOT" "$APP_ENV" "$COMPOSER_ROOT_VERSION" "$SHOPWARE_SKIP_ASSET_INSTALL_CACHE_INVALIDATION" > marker.txt`}, root, env))
	content, err := os.ReadFile(filepath.Join(root, "marker.txt"))
	require.NoError(t, err)
	assert.Equal(t, root+"\nprod\n1.0.0\n1\n", string(content))
	err = executeCIHooks(t.Context(), "test-hooks", []string{"exit 3"}, root, env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hook failed (exit 3)")
}

func TestPrepareComposerAuthMergesShopwarePackagesToken(t *testing.T) {
	t.Setenv("COMPOSER_AUTH", "")
	t.Setenv("SHOPWARE_PACKAGES_TOKEN", "token123")
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "auth.json"),
		[]byte(`{"http-basic":{"example.com":{"username":"u","password":"p"}}}`), 0o600))
	out, err := prepareComposerAuth(t.Context(), root)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	require.Contains(t, parsed, "http-basic")
	assert.Contains(t, parsed["http-basic"], "example.com")
	require.Contains(t, parsed, "bearer")
	bearer, ok := parsed["bearer"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "token123", bearer["packages.shopware.com"])

	out, err = prepareComposerAuth(t.Context(), t.TempDir())
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))
	badRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(badRoot, "auth.json"), []byte("not-json"), 0o600))
	_, err = prepareComposerAuth(t.Context(), badRoot)
	require.Error(t, err)
}

func TestCreateEmptySnippetFolderCreatesGitkeepStubs(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, createEmptySnippetFolder(root))
	for _, dir := range []string{
		"Resources/app/administration/src/app/snippet",
		"Resources/app/administration/src/module/dummy/snippet",
		"Resources/app/administration/src/app/component/dummy/dummy/snippet",
	} {
		assert.FileExists(t, filepath.Join(root, dir, ".gitkeep"))
	}
}

func TestBinCICommand(t *testing.T) {
	cmdExecutor := executor.NewLocal("/project")
	version := binCICommand(t.Context(), cmdExecutor, "--version")
	assert.Equal(t, []string{"php", "bin/ci", "--version"}, version.Cmd.Args)
	assetInstall := binCICommand(t.Context(), cmdExecutor, "asset:install")
	assert.Equal(t, []string{"php", "bin/ci", "asset:install"}, assetInstall.Cmd.Args)
}

func TestRunCommandPreservesExecutorEnv(t *testing.T) {
	cmdExecutor := executor.NewLocal("/project").WithEnv(map[string]string{
		"PROJECT_ROOT": "/project",
		"ADMIN_ROOT":   "/project/vendor/shopware/administration",
	})
	proc := cmdExecutor.NPMCommand(t.Context(), "run", "dev")
	applyTransparentEnv(proc)
	assert.Contains(t, proc.Cmd.Env, "PROJECT_ROOT=/project")
	assert.Contains(t, proc.Cmd.Env, "ADMIN_ROOT=/project/vendor/shopware/administration")
	assert.Contains(t, proc.Cmd.Env, "LOCK_DSN=flock")
}

func TestRunCommandFallsBackToProcessEnv(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_TRANSPARENT_ENV_MARKER", "present")
	proc := &executor.Process{Cmd: exec.CommandContext(t.Context(), "true")}
	require.Nil(t, proc.Cmd.Env)
	applyTransparentEnv(proc)
	assert.Contains(t, proc.Cmd.Env, "SHOPWARE_CLI_TRANSPARENT_ENV_MARKER=present")
	assert.Contains(t, proc.Cmd.Env, "LOCK_DSN=flock")
}

func TestRunCommandKeepsConfiguredStreams(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses cat")
	}
	var out bytes.Buffer
	proc := &executor.Process{Cmd: exec.CommandContext(t.Context(), "cat")}
	proc.Cmd.Stdin = strings.NewReader("from cobra")
	proc.Cmd.Stdout = &out
	require.NoError(t, RunCommand(proc))
	assert.Equal(t, "from cobra", out.String())
}

func TestBuildCleanupPathsAreIndependent(t *testing.T) {
	first := &shop.ConfigBuild{CleanupPaths: []string{"first-only"}}
	second := &shop.ConfigBuild{CleanupPaths: []string{"second-only"}}
	assert.Contains(t, buildCleanupPaths(first), "first-only")
	paths := buildCleanupPaths(second)
	assert.NotContains(t, paths, "first-only")
	assert.Contains(t, paths, "second-only")
	assert.ElementsMatch(t, defaultCleanupPaths, buildCleanupPaths(&shop.ConfigBuild{}))
	paths[0] = "mutated"
	assert.NotContains(t, buildCleanupPaths(second), "mutated")
	assert.Equal(t, []string{"second-only"}, second.CleanupPaths)
}

func TestBuildEnvironment(t *testing.T) {
	env := buildEnvironment(func(string) string { return "" })
	assert.Equal(t, "prod", env["APP_ENV"])
	assert.Equal(t, "1.0.0", env["COMPOSER_ROOT_VERSION"])
	assert.Equal(t, "1", env["SHOPWARE_SKIP_ASSET_INSTALL_CACHE_INVALIDATION"])
	custom := map[string]string{"APP_ENV": "test", "COMPOSER_ROOT_VERSION": "2.0.0"}
	env = buildEnvironment(func(key string) string { return custom[key] })
	assert.Equal(t, "test", env["APP_ENV"])
	assert.Equal(t, "2.0.0", env["COMPOSER_ROOT_VERSION"])
}
