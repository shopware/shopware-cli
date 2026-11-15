package git

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestExportIgnoreMatch(t *testing.T) {
	root := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(root, ".gitattributes"), `# comment
[attr]devonly export-ignore -diff
*.go eol=lf
/tests export-ignore
workers/ export-ignore
*.md export-ignore
README.md -export-ignore
/docs/** export-ignore
src/**/fixtures export-ignore
"spaced name.txt" export-ignore
value.txt export-ignore=yes
[!a-y]ebra.txt	text export-ignore
tools devonly
!negated.txt export-ignore
/vendor-bin/*/composer.lock export-ignore
`)
	testhelper.WriteFile(t, filepath.Join(root, "src", ".gitattributes"), `CHANGELOG.md !export-ignore
/Resources/dev export-ignore
*.snap export-ignore
`)
	testhelper.WriteFile(t, filepath.Join(root, "src", "keep", ".gitattributes"), "*.snap -export-ignore\n[attr]nested export-ignore\nnested.txt nested\n")

	cases := []struct {
		path    string
		isDir   bool
		ignored bool
	}{
		{"tests", true, true},
		{"src/tests", true, false},
		{"workers", true, true},
		{"src/workers", true, true},
		{"workers", false, false},
		{"UPGRADE.md", false, true},
		{"src/deep/UPGRADE.md", false, true},
		{"README.md", false, false},
		{"src/README.md", false, false},
		{"docs", true, false},
		{"docs/index.html", false, true},
		{"docs/a/b.html", false, true},
		{"src/fixtures", true, true},
		{"src/a/b/fixtures", true, true},
		{"fixtures", true, false},
		{"spaced name.txt", false, true},
		{"value.txt", false, false},
		{"zebra.txt", false, true},
		{"aebra.txt", false, false},
		{"tools", true, true},
		{"negated.txt", false, false},
		{"vendor-bin/phpstan/composer.lock", false, true},
		{"vendor-bin/a/b/composer.lock", false, false},
		{"src/CHANGELOG.md", false, false},
		{"CHANGELOG.md", false, true},
		{"src/Resources/dev", true, true},
		{"Resources/dev", true, false},
		{"src/a.snap", false, true},
		{"a.snap", false, false},
		{"src/keep/a.snap", false, false},
		{"src/keep/nested.txt", false, false},
		{"main.go", false, false},
		{".", true, false},
	}

	exportIgnore := NewExportIgnore(root)

	for _, tc := range cases {
		ignored, err := exportIgnore.Match(filepath.FromSlash(tc.path), tc.isDir)
		require.NoError(t, err)
		assert.Equal(t, tc.ignored, ignored, "%s (dir: %v)", tc.path, tc.isDir)
	}
}

func TestExportIgnoreWithoutAttributesFile(t *testing.T) {
	ignored, err := NewExportIgnore(t.TempDir()).Match("tests", true)
	require.NoError(t, err)
	assert.False(t, ignored)
}
