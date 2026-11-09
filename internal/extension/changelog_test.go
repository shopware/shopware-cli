package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-version"
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

func TestChangelogParsingKeepAChangelogWithH2Sections(t *testing.T) {
	input := `# Changelog

## [Unreleased]

## [3.8.0](https://github.com/FriendsOfShopware/FroshPlatformMailArchive/releases/tag/3.8.0) - 2026-10-23

## Added
- Show CC and BCC recipients of the original mail in the mail archive detail view, read from the archived EML ([#160](https://github.com/FriendsOfShopware/FroshPlatformMailArchive/pull/160))
- Add ` + "`frosh:mail-archive:cleanup`" + ` console command to run the archive cleanup on demand; the cleanup is now processed as an async message by the command and the scheduled task ([#119](https://github.com/FriendsOfShopware/FroshPlatformMailArchive/pull/119))

## Fixed
- Bulk resend now sends mails sequentially and reports an error when a send fails, fixing failures on mailers with a concurrent connection limit ([#116](https://github.com/FriendsOfShopware/FroshPlatformMailArchive/pull/116))
`

	content, err := parseMarkdownChangelog(input)
	assert.NoError(t, err)

	expected := "<h2>Added</h2>\n<ul>\n" +
		"<li>Show CC and BCC recipients of the original mail in the mail archive detail view, read from the archived EML (<a href=\"https://github.com/FriendsOfShopware/FroshPlatformMailArchive/pull/160\">#160</a>)</li>\n" +
		"<li>Add <code>frosh:mail-archive:cleanup</code> console command to run the archive cleanup on demand; the cleanup is now processed as an async message by the command and the scheduled task (<a href=\"https://github.com/FriendsOfShopware/FroshPlatformMailArchive/pull/119\">#119</a>)</li>\n" +
		"</ul>\n<h2>Fixed</h2>\n<ul>\n" +
		"<li>Bulk resend now sends mails sequentially and reports an error when a send fails, fixing failures on mailers with a concurrent connection limit (<a href=\"https://github.com/FriendsOfShopware/FroshPlatformMailArchive/pull/116\">#116</a>)</li>\n" +
		"</ul>\n"

	assert.Equal(t, map[string]string{"3.8.0": expected}, content)
}

func TestChangelogParsingKeepAChangelogSpec(t *testing.T) {
	input := `# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Unreleased feature that should not appear in released versions

## [1.1.0] - 2023-03-05

### Added

- Arabic translation (#444).
- v1.1 French translation.

### Fixed

- Improve French translation (#377).

## [1.0.0] - 2017-06-20 [YANKED]

### Removed

- Old deprecated config

[unreleased]: https://github.com/olivierlacan/keep-a-changelog/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/olivierlacan/keep-a-changelog/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/olivierlacan/keep-a-changelog/releases/tag/v1.0.0
`

	content, err := parseMarkdownChangelog(input)
	assert.NoError(t, err)

	assert.Equal(t, map[string]string{
		"1.1.0": "<h3>Added</h3>\n<ul>\n<li>Arabic translation (#444).</li>\n<li>v1.1 French translation.</li>\n</ul>\n<h3>Fixed</h3>\n<ul>\n<li>Improve French translation (#377).</li>\n</ul>\n",
		"1.0.0": "<h3>Removed</h3>\n<ul>\n<li>Old deprecated config</li>\n</ul>\n",
	}, content)
}

func TestChangelogParsingKeepAChangelogReferenceLinksAndVariants(t *testing.T) {
	input := "# Changelog\r\n\r\n" +
		"## [Unreleased]\r\n\r\n" +
		"## [v2.0.0-RC1](https://example.com/compare/v1.0.0...v2.0.0-RC1) (2026-10-23)\r\n\r\n" +
		"### Changed\r\n" +
		"- See [migration guide][migration]\r\n\r\n" +
		"[migration]: https://example.com/migration\r\n"

	content, err := parseMarkdownChangelog(input)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		"2.0.0-RC1": "<h3>Changed</h3>\n<ul>\n<li>See <a href=\"https://example.com/migration\">migration guide</a></li>\n</ul>\n",
	}, content)
}

func TestChangelogParsingReferenceWithoutSpace(t *testing.T) {
	input := "## [2.0.0]\n\nSee [guide].\n\n## [1.0.0]\n\nOlder release.\n\n[guide]:https://example.com/guide\n"
	content, err := parseMarkdownChangelog(input)
	require.NoError(t, err)

	assert.Equal(t, "<p>See <a href=\"https://example.com/guide\">guide</a>.</p>\n", content["2.0.0"])
	assert.Equal(t, "<p>Older release.</p>\n", content["1.0.0"])
}

func TestChangelogParsingEscapedHeadings(t *testing.T) {
	input := `## \[2.0.0\] - 2024-01-01

Released feature.

## \[Unreleased\]

Future feature.

## \[1.0.0\]

Older release.
`
	content, err := parseMarkdownChangelog(input)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"2.0.0": "<p>Released feature.</p>\n",
		"1.0.0": "<p>Older release.</p>\n",
	}, content)
}

func TestChangelogParsingMarkdownBlocks(t *testing.T) {
	input := "[2.0.0]\n-------\n\n" +
		"    ## [9.0.0]\n\n" +
		"> ## [8.0.0]\n> Quoted example.\n\n" +
		"See [guide].\n\n## [1.0.0] ##\n\nOlder release.\n\n" +
		"[guide]:\n  https://example.com/guide\n  \"Migration guide\"\n"
	content, err := parseMarkdownChangelog(input)
	require.NoError(t, err)

	require.Len(t, content, 2)
	assert.Contains(t, content["2.0.0"], "<pre><code>## [9.0.0]\n</code></pre>")
	assert.Contains(t, content["2.0.0"], "<blockquote>\n<h2>[8.0.0]</h2>\n<p>Quoted example.</p>\n</blockquote>")
	assert.Contains(t, content["2.0.0"], "<a href=\"https://example.com/guide\" title=\"Migration guide\">guide</a>")
	assert.Equal(t, "<p>Older release.</p>\n", content["1.0.0"])
}

func TestChangelogParsingFencedExamples(t *testing.T) {
	for _, tt := range []struct {
		name  string
		open  string
		inner string
		close string
	}{
		{name: "backticks", open: "```markdown", close: "```"},
		{name: "tildes", open: "~~~markdown", close: "~~~"},
		{name: "indented fences", open: "   ```markdown", close: "   ```\t"},
		{name: "longer closing fence", open: "```", close: "`````"},
		{name: "short fence stays inside", open: "````", inner: "```\n", close: "````"},
		{name: "different fence stays inside", open: "```", inner: "~~~\n", close: "```"},
		{name: "closing fence with text stays inside", open: "```", inner: "``` text\n", close: "```"},
		{name: "indented closing fence stays inside", open: "```", inner: "    ```\n", close: "```"},
		{name: "tilde info allows backticks", open: "~~~example `code`", close: "~~~"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := "## [3.0.0]\n\n" + tt.open + "\n" + tt.inner +
				"## [2.0.0]\n## [Unreleased]\n[guide]:https://example.com/example\n" +
				tt.close + "\n\nSee [guide].\n\n## [1.0.0]\n\nOlder release.\n\n[guide]:https://example.com/guide\n"
			content, err := parseMarkdownChangelog(input)
			require.NoError(t, err)

			require.Len(t, content, 2)
			assert.Contains(t, content["3.0.0"], "## [2.0.0]\n## [Unreleased]\n[guide]:https://example.com/example\n")
			assert.Contains(t, content["3.0.0"], "<p>See <a href=\"https://example.com/guide\">guide</a>.</p>")
			assert.Equal(t, "<p>Older release.</p>\n", content["1.0.0"])
		})
	}
}

func TestParseExtensionMarkdownChangelogKeepAChangelog(t *testing.T) {
	dir := t.TempDir()
	changelogContent := `# Changelog

## [Unreleased]

## [3.8.0](https://github.com/FriendsOfShopware/FroshPlatformMailArchive/releases/tag/3.8.0) - 2026-10-23

## Added
- Show CC and BCC recipients of the original mail in the mail archive detail view, read from the archived EML ([#160](https://github.com/FriendsOfShopware/FroshPlatformMailArchive/pull/160))

## Fixed
- Bulk resend now sends mails sequentially ([#116](https://github.com/FriendsOfShopware/FroshPlatformMailArchive/pull/116))
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(changelogContent), 0o644))

	ext := &mockExtension{
		path:       dir,
		extVersion: version.Must(version.NewVersion("v3.8.0")),
	}

	changelog, err := parseExtensionMarkdownChangelog(ext)
	require.NoError(t, err)

	expected := "<h2>Added</h2>\n<ul>\n" +
		"<li>Show CC and BCC recipients of the original mail in the mail archive detail view, read from the archived EML (<a href=\"https://github.com/FriendsOfShopware/FroshPlatformMailArchive/pull/160\">#160</a>)</li>\n" +
		"</ul>\n<h2>Fixed</h2>\n<ul>\n" +
		"<li>Bulk resend now sends mails sequentially (<a href=\"https://github.com/FriendsOfShopware/FroshPlatformMailArchive/pull/116\">#116</a>)</li>\n" +
		"</ul>\n"

	assert.Equal(t, expected, changelog.English)
	assert.Equal(t, expected, changelog.German)
	assert.Equal(t, expected, changelog.Changelogs["en-GB"])
}
