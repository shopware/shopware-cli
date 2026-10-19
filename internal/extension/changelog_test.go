package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChangelogParsing(t *testing.T) {
	content, err := parseMarkdownChangelog("# 1.0.0\n\n- Test\n- Test2\n# 2.0.0\n- Test3\n- Test4\n")
	assert.NoError(t, err)

	assert.Equal(t, "<ul>\n<li>Test</li>\n<li>Test2</li>\n</ul>\n", content["1.0.0"])
	assert.Equal(t, "<ul>\n<li>Test3</li>\n<li>Test4</li>\n</ul>\n", content["2.0.0"])
}

func TestChangelogParsingWhitespaces(t *testing.T) {
	content, err := parseMarkdownChangelog("# 1.0.0\n \n- Test\n- Test2\n# 2.0.0\n- Test3\n - Test4\n")
	assert.NoError(t, err)

	assert.Equal(t, "<ul>\n<li>Test</li>\n<li>Test2</li>\n</ul>\n", content["1.0.0"])
	assert.Equal(t, "<ul>\n<li>Test3</li>\n<li>Test4</li>\n</ul>\n", content["2.0.0"])
}

func TestChangelogParsingSubHeadings(t *testing.T) {
	content, err := parseMarkdownChangelog("# 1.0.0\n\n## Added\n\n- New feature\n\n## Fixed\n\n- A bug\n\n# 0.9.0\n\n- Old\n")
	assert.NoError(t, err)

	assert.Len(t, content, 2)
	assert.Contains(t, content["1.0.0"], `<span class="h2" id="added">Added</span>`)
	assert.Contains(t, content["1.0.0"], "<li>New feature</li>")
	assert.Contains(t, content["1.0.0"], `<span class="h2" id="fixed">Fixed</span>`)
	assert.Contains(t, content["1.0.0"], "<li>A bug</li>")
	assert.Equal(t, "<ul>\n<li>Old</li>\n</ul>\n", content["0.9.0"])
}

func TestChangelogParsingKeepAChangelogStyle(t *testing.T) {
	markdown := `# Changelog

All notable changes to this project are documented here.

## [Unreleased]

- Not released yet

## [1.0.0] - 2024-01-31

### Added

- New feature with **emphasis** and a [link](https://example.com)

### Fixed

1. First fix
2. Second fix

## [0.9.0] - 2023-12-01

- Old change

[1.0.0]: https://example.com/releases/1.0.0
[0.9.0]: https://example.com/releases/0.9.0
`
	content, err := parseMarkdownChangelog(markdown)
	assert.NoError(t, err)

	assert.Len(t, content, 2)
	assert.Contains(t, content["1.0.0"], `<span class="h3" id="added">Added</span>`)
	assert.Contains(t, content["1.0.0"], "<strong>emphasis</strong>")
	assert.Contains(t, content["1.0.0"], `<a href="https://example.com">link</a>`)
	assert.Contains(t, content["1.0.0"], "<li>First fix</li>")
	assert.NotContains(t, content["1.0.0"], "Not released yet")
	assert.NotContains(t, content["1.0.0"], "All notable changes")
	assert.Equal(t, "<ul>\n<li>Old change</li>\n</ul>\n", content["0.9.0"])
}

func TestChangelogParsingVersionHeadingDecorations(t *testing.T) {
	content, err := parseMarkdownChangelog("# v1.2.0\n\n- A\n\n# [1.1.0] - 2024-01-01\n\n- B\n\n# 1.0.0 (2023-12-01)\n\n- C\n")
	assert.NoError(t, err)

	assert.Len(t, content, 3)
	assert.Contains(t, content["1.2.0"], "<li>A</li>")
	assert.Contains(t, content["1.1.0"], "<li>B</li>")
	assert.Contains(t, content["1.0.0"], "<li>C</li>")
}

func TestChangelogParsingAdvancedMarkdown(t *testing.T) {
	markdown := "# 1.0.0\n\nA paragraph explaining the release.\n\n> A note for upgraders\n\n```\n# not a heading\n- not a list item of the changelog\n```\n\n- Real change\n\n# About this changelog\n\nThis text is not part of any release.\n"
	content, err := parseMarkdownChangelog(markdown)
	assert.NoError(t, err)

	assert.Len(t, content, 1)
	assert.Contains(t, content["1.0.0"], "<p>A paragraph explaining the release.</p>")
	assert.Contains(t, content["1.0.0"], "<blockquote>")
	assert.Contains(t, content["1.0.0"], "# not a heading")
	assert.Contains(t, content["1.0.0"], "<li>Real change</li>")
	assert.NotContains(t, content["1.0.0"], "not part of any release")
}

func TestParseExtensionMarkdownChangelogAdvancedStyle(t *testing.T) {
	dir := t.TempDir()
	markdown := "# Changelog\n\n## [Unreleased]\n\n- Not released yet\n\n## [1.0.0] - 2024-01-31\n\n### Added\n\n- New feature\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CHANGELOG_en-GB.md"), []byte(markdown), 0o644))

	changelog, err := parseExtensionMarkdownChangelog(&mockExtension{path: dir})
	assert.NoError(t, err)
	require.NotNil(t, changelog)
	assert.Contains(t, changelog.English, `<span class="h3" id="added">Added</span>`)
	assert.Contains(t, changelog.English, "<li>New feature</li>")
	assert.NotContains(t, changelog.English, "Not released yet")
	assert.Equal(t, changelog.English, changelog.German)
}

func TestChangelogParsingHeadingMentioningVersionIsNoRelease(t *testing.T) {
	content, err := parseMarkdownChangelog("# 1.0.0\n\n## Migrating to 2.0.0\n\n- Read the guide\n")
	assert.NoError(t, err)

	assert.Len(t, content, 1)
	assert.Contains(t, content["1.0.0"], "Migrating to 2.0.0")
	assert.Contains(t, content["1.0.0"], "<li>Read the guide</li>")
}
