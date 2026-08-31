package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupTcpdfKeepsOnlyCourierAndHelvetica(t *testing.T) {
	root := t.TempDir()
	fonts := filepath.Join(root, "vendor", "tecnickcom", "tcpdf", "fonts")
	require.NoError(t, os.MkdirAll(fonts, 0o755))
	// foo.z falls to the general keep-list rule; the file named exactly .z
	// has its own branch.
	for _, name := range []string{"helvetica.php", "courier_bold.php", "times.php", "foo.z", ".z"} {
		require.NoError(t, os.WriteFile(filepath.Join(fonts, name), []byte("x"), 0o644))
	}

	require.NoError(t, cleanupTcpdf(root, t.Context()))

	assert.FileExists(t, filepath.Join(fonts, "helvetica.php"))
	assert.FileExists(t, filepath.Join(fonts, "courier_bold.php"))
	assert.NoFileExists(t, filepath.Join(fonts, "times.php"))
	assert.NoFileExists(t, filepath.Join(fonts, "foo.z"))
	assert.NoFileExists(t, filepath.Join(fonts, ".z"))

	// A project without the tcpdf vendor dir is a no-op.
	require.NoError(t, cleanupTcpdf(t.TempDir(), t.Context()))
}

func TestExecuteCIHooksRunsInRootWithProjectRootEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hooks run through sh -c")
	}

	root := t.TempDir()
	require.NoError(t, executeCIHooks(t.Context(), "test-hooks", []string{`printf %s "$PROJECT_ROOT" > marker.txt`}, root))

	content, err := os.ReadFile(filepath.Join(root, "marker.txt"))
	require.NoError(t, err)
	assert.Equal(t, root, string(content))

	err = executeCIHooks(t.Context(), "test-hooks", []string{"exit 3"}, root)
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

	// Missing auth.json still yields a valid env-only result.
	out, err = prepareComposerAuth(t.Context(), t.TempDir())
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(out), &parsed))

	// Malformed auth.json errors.
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
