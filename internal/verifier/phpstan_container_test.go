package verifier

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhpStanBaseConfigPrefersProjectConfig(t *testing.T) {
	root := t.TempDir()
	toolDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "phpstan.neon.dist"), []byte("parameters:\n"), 0o644))

	got := phpStanBaseConfig(ToolConfig{RootDir: root, ToolDirectory: toolDir})

	assert.Equal(t, path.Join(root, "phpstan.neon.dist"), got)
}

func TestPhpStanBaseConfigFallsBackToDefault(t *testing.T) {
	root := t.TempDir()
	toolDir := t.TempDir()

	got := phpStanBaseConfig(ToolConfig{RootDir: root, ToolDirectory: toolDir})

	assert.Equal(t, path.Join(toolDir, "php", "configs", "phpstan.neon"), got)
}

func TestWritePhpStanContainerConfig(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "phpstan.neon")
	xml := filepath.Join(root, "container.xml")
	require.NoError(t, os.WriteFile(base, []byte("parameters:\n    level: 5\n"), 0o644))
	require.NoError(t, os.WriteFile(xml, []byte("<container/>\n"), 0o644))

	configFile, cleanup, err := writePhpStanContainerConfig(base, xml)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	content, err := os.ReadFile(configFile)
	require.NoError(t, err)

	absBase, err := filepath.Abs(base)
	require.NoError(t, err)
	absXML, err := filepath.Abs(xml)
	require.NoError(t, err)

	assert.Contains(t, string(content), "includes:")
	assert.Contains(t, string(content), absBase)
	assert.Contains(t, string(content), absXML)
	assert.Contains(t, string(content), "containerXmlPath:")
}

func TestNeonStringEscapesQuotes(t *testing.T) {
	assert.Equal(t, `'/tmp/it''s.neon'`, neonString("/tmp/it's.neon"))
}

func TestDumpShopwareContainer(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("php"); err != nil {
		t.Skip("php is not installed")
	}

	t.Run("skip", func(t *testing.T) {
		t.Parallel()
		root, toolDir := dumpContainerFixture(t, "<?php fwrite(STDERR, \"no shopware\\n\"); exit(3);\n")

		xml, err := dumpShopwareContainer(t.Context(), ToolConfig{RootDir: root, ToolDirectory: toolDir})

		require.NoError(t, err)
		assert.Empty(t, xml)
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		root, toolDir := dumpContainerFixture(t, "<?php fwrite(STDERR, \"theme.repository missing\\n\"); exit(1);\n")

		xml, err := dumpShopwareContainer(t.Context(), ToolConfig{RootDir: root, ToolDirectory: toolDir})

		require.Error(t, err)
		assert.Empty(t, xml)
		assert.Contains(t, err.Error(), "theme.repository missing")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		script := `<?php
$path = $argv[1] . '/container.xml';
file_put_contents($path, "<container/>\n");
echo $path, "\n";
`
		root, toolDir := dumpContainerFixture(t, script)

		xml, err := dumpShopwareContainer(t.Context(), ToolConfig{RootDir: root, ToolDirectory: toolDir})

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(root, "container.xml"), xml)
	})
}

func dumpContainerFixture(t *testing.T, script string) (string, string) {
	t.Helper()

	root := t.TempDir()
	toolDir := t.TempDir()
	configDir := filepath.Join(toolDir, "php", "configs")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "dump-container.php"), []byte(script), 0o644))

	return root, toolDir
}
