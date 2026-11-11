package verifier

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestCanceledToolsDoNotStartWork(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	results := RunTools(ctx, ToolList[Tool]{testTool{name: "one"}, testTool{name: "two"}}, nil, ToolConfig{}, nil, func(Tool, *ToolExecution) ToolSummary {
		t.Error("started a tool after cancellation")
		return ToolSummary{}
	})
	require.Len(t, results, 2)
	for _, result := range results {
		require.NotEmpty(t, result.Errors)
		assert.ErrorIs(t, result.Errors[0], context.Canceled)
	}
	runDirectories(ctx, []string{"one", "two"}, func(string) {
		t.Error("started a directory after cancellation")
	})
}

func TestExtensionVersionDiscoveryUsesCommandContext(t *testing.T) {
	previous := getShopwareVersions
	t.Cleanup(func() { getShopwareVersions = previous })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	getShopwareVersions = func(received context.Context) ([]string, error) {
		cancel()
		return nil, received.Err()
	}
	ext, err := extension.GetExtensionByFolder(ctx, testhelper.NewPlugin(t, "Example"))
	require.NoError(t, err)
	_, err = ConvertExtensionToToolConfig(ctx, ext)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCanceledXMLFixPreservesFiles(t *testing.T) {
	root := testhelper.NewPlugin(t, "Example")
	xmlPath := filepath.Join(root, "src", "Resources", "config", "services.xml")
	require.NoError(t, os.MkdirAll(filepath.Dir(xmlPath), 0o755))
	require.NoError(t, os.WriteFile(xmlPath, []byte("<container/>"), 0o644))
	ext, err := extension.GetExtensionByFolder(t.Context(), root)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	execution := NewToolExecution(nil)
	SymfonyXMLConverter{}.Fix(ctx, ToolConfig{Extensions: []extension.Extension{ext}}, execution, false)
	assert.FileExists(t, xmlPath)
	assert.NoFileExists(t, filepath.Join(filepath.Dir(xmlPath), "services.yaml"))
}

func TestCanceledInstallationIsNotReused(t *testing.T) {
	cache := t.TempDir()
	ready := filepath.Join(cache, "ready")
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", cache)
	t.Setenv("SHOPWARE_CLI_TOOLS_DIR", "")
	t.Setenv("PHP_BINARY", "")
	t.Setenv("INSTALL_READY", ready)
	t.Setenv("CANCEL_INSTALL", "1")
	previous := toolDirectory
	t.Cleanup(func() { setToolDirectory(previous) })
	fakeTools(t, map[string]string{
		"php":  "printf 'PHP 8.2.0 (cli)'",
		"node": "printf 'v20.0.0'",
		"composer": `if [ "$CANCEL_INSTALL" = "1" ]; then
  : > "$INSTALL_READY"
  exec /bin/sleep 30
fi`,
		"npm": ": > installed",
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- SetupTools(ctx, "cancellation-test") }()
	require.Eventually(t, func() bool {
		_, err := os.Stat(ready)
		return err == nil
	}, 5*time.Second, 10*time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("tool installation did not stop")
	}
	installed := filepath.Join(cache, "tools", "cancellation-test")
	assert.NoDirExists(t, installed)

	t.Setenv("CANCEL_INSTALL", "")
	require.NoError(t, SetupTools(t.Context(), "cancellation-test"))
	assert.FileExists(t, filepath.Join(installed, "js", "installed"))
	assert.Equal(t, installed, GetToolDirectory())
}
