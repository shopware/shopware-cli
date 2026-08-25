package extension

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cp "github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
)

func TestExecuteHooksEnvContractAndFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hooks run through sh -c")
	}

	dir := writePluginFixture(t)
	ext, err := extension.GetExtensionByFolder(t.Context(), dir)
	require.NoError(t, err)

	extDir := t.TempDir()
	hooks := []string{
		`printf %s "$EXTENSION_DIR" > env.txt`,
		`printf %s "$ORIGINAL_EXTENSION_DIR" > orig.txt`,
	}
	require.NoError(t, executeHooks(t.Context(), ext, hooks, extDir))

	// Hooks run inside extDir with both directories in the environment.
	envContent, err := os.ReadFile(filepath.Join(extDir, "env.txt"))
	require.NoError(t, err)
	assert.Equal(t, extDir, string(envContent))

	origContent, err := os.ReadFile(filepath.Join(extDir, "orig.txt"))
	require.NoError(t, err)
	assert.Equal(t, ext.GetPath(), string(origContent))

	err = executeHooks(t.Context(), ext, []string{"exit 3"}, extDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 3")
}

func TestCopyOptionsSkipSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs extra privileges on windows")
	}

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "real.txt"), []byte("data"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(src, "real.txt"), filepath.Join(src, "link.txt")))

	dst := filepath.Join(t.TempDir(), "out")
	require.NoError(t, cp.Copy(src, dst, copyOptions()))

	assert.FileExists(t, filepath.Join(dst, "real.txt"))
	assert.NoFileExists(t, filepath.Join(dst, "link.txt"))
}
