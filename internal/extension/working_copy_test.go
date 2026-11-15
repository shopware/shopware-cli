package extension

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func writeExportIgnoreFixture(t *testing.T, dir string) {
	t.Helper()

	files := map[string]string{
		".gitattributes": `[attr]devonly export-ignore
/tests export-ignore
workers/ export-ignore
*.md export-ignore
README.md -export-ignore
/docs/** export-ignore
src/**/fixtures export-ignore
"spaced name.txt" export-ignore
value.txt export-ignore=yes
[!a-y]ebra.txt export-ignore
tools devonly
/vendor-bin/*/composer.lock export-ignore
/.gitattributes export-ignore
`,
		"src/.gitattributes": `CHANGELOG.md !export-ignore
/Resources/dev export-ignore
*.snap export-ignore
`,
		"src/keep/.gitattributes":          "*.snap -export-ignore\n",
		"composer.json":                    "{}",
		"README.md":                        "x",
		"UPGRADE.md":                       "x",
		"CHANGELOG.md":                     "x",
		"tests/FooTest.php":                "x",
		"workers/worker.js":                "x",
		"src/workers/worker.js":            "x",
		"src/tests/keep.php":               "x",
		"docs/index.html":                  "x",
		"docs/a/b.html":                    "x",
		"src/fixtures/a.json":              "x",
		"src/a/b/fixtures/a.json":          "x",
		"fixtures/a.json":                  "x",
		"spaced name.txt":                  "x",
		"value.txt":                        "x",
		"zebra.txt":                        "x",
		"aebra.txt":                        "x",
		"tools/build.sh":                   "x",
		"vendor-bin/phpstan/composer.lock": "x",
		"vendor-bin/phpstan/composer.json": "x",
		"src/CHANGELOG.md":                 "x",
		"src/deep/UPGRADE.md":              "x",
		"src/Resources/dev/a.js":           "x",
		"src/Resources/public/a.js":        "x",
		"src/a.snap":                       "x",
		"src/keep/a.snap":                  "x",
		"src/Foo.php":                      "x",
	}

	for name, content := range files {
		testhelper.WriteFile(t, filepath.Join(dir, filepath.FromSlash(name)), content)
	}
}

func listFiles(t *testing.T, dir string) []string {
	t.Helper()

	var files []string

	require.NoError(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel := filepath.ToSlash(strings.TrimPrefix(path, dir+string(os.PathSeparator)))
		if entry.IsDir() {
			if rel == ".git" {
				return filepath.SkipDir
			}

			return nil
		}

		files = append(files, rel)

		return nil
	}))

	return files
}

func TestCopyWorkingCopyMatchesGitArchive(t *testing.T) {
	source := t.TempDir()
	writeExportIgnoreFixture(t, source)

	gitRun(t, source, "init")
	gitRun(t, source, "config", "commit.gpgsign", "false")
	gitRun(t, source, "add", ".")
	gitRun(t, source, "commit", "-m", "initial")

	viaGit := t.TempDir()
	_, err := GitCopyFolder(t.Context(), source, viaGit, "HEAD")
	require.NoError(t, err)

	viaCopy := t.TempDir()
	require.NoError(t, CopyWorkingCopy(source, viaCopy, &Config{CompatibilityDate: CompatibilityExportIgnore}))

	expected := listFiles(t, viaGit)
	assert.Contains(t, expected, "src/Foo.php")
	assert.NotContains(t, expected, "workers/worker.js")
	assert.ElementsMatch(t, expected, listFiles(t, viaCopy))
}

func TestCopyWorkingCopyKeepsExportIgnoredFilesBeforeCompatibilityDate(t *testing.T) {
	source := t.TempDir()
	writeExportIgnoreFixture(t, source)

	target := t.TempDir()
	require.NoError(t, CopyWorkingCopy(source, target, &Config{CompatibilityDate: "2026-11-14"}))

	assert.ElementsMatch(t, listFiles(t, source), listFiles(t, target))
}

func TestCopyWorkingCopySkipsSymlinks(t *testing.T) {
	source := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(source, "composer.json"), "{}")
	require.NoError(t, os.Symlink(filepath.Join(source, "composer.json"), filepath.Join(source, "link.json")))

	target := t.TempDir()
	require.NoError(t, CopyWorkingCopy(source, target, &Config{CompatibilityDate: CompatibilityExportIgnore}))

	assert.ElementsMatch(t, []string{"composer.json"}, listFiles(t, target))
}
