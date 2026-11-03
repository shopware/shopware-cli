package verifier

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/validation"
)

// fakePrettier reports a file as unformatted when the first ignore file does
// not resolve to the process' real working directory, mirroring how Prettier
// matches ignore patterns relative to the ignore file's location.
const fakePrettier = `const path = require("path");
const ignore = process.argv[process.argv.indexOf("--ignore-path") + 1];
if (path.dirname(path.resolve(ignore)) !== process.cwd()) {
  console.log("Resources/public/compiled.js");
  process.exit(1);
}
`

func TestPrettierAppliesBuiltinIgnoreInSymlinkedDirectory(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	toolDir := t.TempDir()
	bin := filepath.Join(toolDir, "js", "node_modules", ".bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "prettier"), []byte(fakePrettier), 0o644))

	root := t.TempDir()
	realDir := filepath.Join(root, "custom", "static-plugins", "MyPlugin", "src")
	require.NoError(t, os.MkdirAll(realDir, 0o755))
	linkDir := filepath.Join(root, "vendor", "acme", "my-plugin")
	require.NoError(t, os.MkdirAll(filepath.Dir(linkDir), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(root, "custom", "static-plugins", "MyPlugin"), linkDir))

	var findings []validation.CheckResult
	execution := NewToolExecution(func(results []validation.CheckResult) bool {
		findings = append(findings, results...)
		return true
	})
	Prettier{}.Format(t.Context(), ToolConfig{
		ToolDirectory:     toolDir,
		RootDir:           root,
		SourceDirectories: []string{filepath.Join(linkDir, "src")},
	}, execution, true)

	assert.Empty(t, execution.snapshot().Errors)
	assert.Empty(t, findings)
	entries, err := os.ReadDir(realDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "temporary ignore file must be removed")
}
