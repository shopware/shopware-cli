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

// fakePrettier lists every file below its working directory that is not
// matched by the first ignore file. Like Prettier, it resolves the ignore file
// against the real working directory and matches patterns relative to the
// ignore file's directory. It supports the pattern forms used by ignoredPaths:
// "dir/**" and exact file names.
const fakePrettier = `const fs = require("fs");
const path = require("path");
const ignore = process.argv[process.argv.indexOf("--ignore-path") + 1];
const ignoreDir = path.dirname(path.resolve(ignore));
const patterns = fs.readFileSync(ignore, "utf8").split("\n").map((p) => p.trim()).filter(Boolean);
const isIgnored = (rel) => patterns.some((p) => (p.endsWith("/**") ? rel.startsWith(p.slice(0, -2)) : rel === p));
const reported = [];
const walk = (dir) => {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const file = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walk(file);
    } else if (!entry.name.startsWith(".shopware-cli-prettier-ignore-")) {
      const rel = path.relative(ignoreDir, file).split(path.sep).join("/");
      if (!isIgnored(rel)) {
        reported.push(path.relative(process.cwd(), file));
      }
    }
  }
};
walk(process.cwd());
if (reported.length > 0) {
  console.log(reported.join("\n"));
  process.exit(1);
}
`

// TestPrettierAppliesBuiltinIgnoreInSymlinkedDirectory checks that the
// built-in ignore list still applies when the source directory is reached
// through a symlink, as with Composer path repositories below vendor/.
func TestPrettierAppliesBuiltinIgnoreInSymlinkedDirectory(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	toolDir := t.TempDir()
	bin := filepath.Join(toolDir, "js", "node_modules", ".bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "prettier"), []byte(fakePrettier), 0o644))

	root := t.TempDir()
	pluginDir := filepath.Join(root, "custom", "static-plugins", "MyPlugin")
	realDir := filepath.Join(pluginDir, "src")
	for _, file := range []string{
		"Resources/app/storefront/dist/storefront/js/my-plugin.js",
		"Resources/public/administration/assets/my-plugin.js",
		"Resources/app/storefront/src/main.js",
	} {
		file = filepath.Join(realDir, filepath.FromSlash(file))
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
		require.NoError(t, os.WriteFile(file, []byte("const a=1\n"), 0o644))
	}
	linkDir := filepath.Join(root, "vendor", "acme", "my-plugin")
	require.NoError(t, os.MkdirAll(filepath.Dir(linkDir), 0o755))
	require.NoError(t, os.Symlink(pluginDir, linkDir))

	var findings []string
	execution := NewToolExecution(func(results []validation.CheckResult) bool {
		for _, result := range results {
			findings = append(findings, result.Path)
		}
		return true
	})
	Prettier{}.Format(t.Context(), ToolConfig{
		ToolDirectory:     toolDir,
		RootDir:           root,
		SourceDirectories: []string{filepath.Join(linkDir, "src")},
	}, execution, true)

	assert.Empty(t, execution.snapshot().Errors)
	assert.ElementsMatch(t, []string{"vendor/acme/my-plugin/src/Resources/app/storefront/src/main.js"}, findings)
	ignoreFiles, err := filepath.Glob(filepath.Join(realDir, ".shopware-cli-prettier-ignore-*"))
	require.NoError(t, err)
	assert.Empty(t, ignoreFiles, "temporary ignore file must be removed")
}
