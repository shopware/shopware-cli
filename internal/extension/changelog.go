package extension

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/shopware/shopware-cli/internal/markdown"
)

func parseMarkdownChangelogInPath(path string) (map[string]map[string]string, error) {
	files, err := filepath.Glob(path + "/CHANGELOG*.md")
	if err != nil {
		return nil, err
	}

	changelogs := make(map[string]map[string]string)

	for _, file := range files {
		language := strings.Trim(strings.ReplaceAll(strings.ReplaceAll(filepath.Base(file), "CHANGELOG", ""), ".md", ""), "_")

		if len(language) == 0 {
			language = "en-GB"
		}

		content, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("cannot read changelog %s: %w", file, err)
		}

		changelogs[language], err = parseMarkdownChangelog(string(content))

		if err != nil {
			return nil, fmt.Errorf("cannot parse changelog %s: %w", file, err)
		}
	}

	return changelogs, nil
}

var (
	// ATX headings require a space after the opening hashes. The legacy
	// Shopware format also wrote version headings without one ("#1.0.0"),
	// which the second pattern keeps accepting for version headings only.
	headingPattern       = regexp.MustCompile(`^(#{1,6})[ \t]+(.*?)[ \t]*$`)
	legacyHeadingPattern = regexp.MustCompile(`^(#{1,6})(.*\S)[ \t]*$`)
	versionPattern       = regexp.MustCompile(`^\d+\.\d+(?:\.\d+)?(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
	fencePattern         = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
)

// changelogVersion extracts the version from a heading text, reporting
// whether the heading announces a release at all. Since Shopware itself no
// longer parses these files (it did up to 6.5, which is why only a plain
// "# x.y.z" followed by list items used to be safe), a version heading may
// carry the decorations common in "Keep a Changelog" style files: a leading
// "v", brackets, a link target, or a trailing date. Anything else, such as
// "Added" or "Migrating to 2.0.0", is not a release heading.
func changelogVersion(heading string) (string, bool) {
	// Strip the closing sequence of an ATX heading ("## 1.0.0 ##").
	heading = strings.TrimSpace(heading)
	if idx := strings.LastIndex(heading, " #"); idx >= 0 && strings.Trim(heading[idx+1:], "#") == "" {
		heading = strings.TrimSpace(heading[:idx])
	}

	candidate := ""
	rest := ""

	if strings.HasPrefix(heading, "[") {
		end := strings.Index(heading, "]")
		if end < 0 {
			return "", false
		}
		candidate = heading[1:end]
		rest = strings.TrimSpace(heading[end+1:])
		// A Markdown link target directly after the bracketed version
		// ("[1.0.0](https://…)") belongs to the version, not the notes.
		if strings.HasPrefix(rest, "(") {
			if close := strings.Index(rest, ")"); close >= 0 {
				rest = strings.TrimSpace(rest[close+1:])
			}
		}
	} else {
		fields := strings.Fields(heading)
		if len(fields) == 0 {
			return "", false
		}
		candidate = fields[0]
		if (strings.EqualFold(candidate, "version") || strings.EqualFold(candidate, "release")) && len(fields) > 1 {
			candidate = fields[1]
			fields = fields[1:]
		}
		if len(fields) > 1 {
			rest = fields[1]
		}
	}

	// After the version only a separator introducing a date or a remark
	// may follow ("1.0.0 - 2024-01-01", "1.0.0 (2024-01-01)").
	if rest != "" {
		separator := []rune(rest)[0]
		if !strings.ContainsRune("-–—(:", separator) {
			return "", false
		}
	}

	candidate = strings.TrimPrefix(strings.TrimPrefix(candidate, "v"), "V")
	if !versionPattern.MatchString(candidate) {
		return "", false
	}

	return candidate, true
}

// parseHeading returns the level and text of an ATX heading line.
func parseHeading(line string) (level int, text string, ok bool) {
	if match := headingPattern.FindStringSubmatch(line); match != nil {
		return len(match[1]), match[2], true
	}
	// Without a space after the hashes only a version can follow, every
	// other line ("#hashtag") is plain text as in Markdown.
	if match := legacyHeadingPattern.FindStringSubmatch(line); match != nil {
		if _, isVersion := changelogVersion(match[2]); isVersion {
			return len(match[1]), match[2], true
		}
	}

	return 0, "", false
}

func parseMarkdownChangelog(content string) (map[string]string, error) {
	rawVersions := make(map[string]string)
	currentVersion := ""
	currentLevel := 0
	collecting := false
	var versionText strings.Builder

	flush := func() {
		if !collecting {
			return
		}
		text := strings.TrimSpace(versionText.String())
		if existing, ok := rawVersions[currentVersion]; ok && existing != "" && text != "" {
			text = existing + "\n\n" + text
		}
		rawVersions[currentVersion] = text
		versionText.Reset()
		collecting = false
	}

	inFence := false
	var fenceChar byte
	fenceLen := 0

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSuffix(line, "\r")

		// Headings inside a fenced code block are sample code, not
		// release separators.
		if match := fencePattern.FindStringSubmatch(line); match != nil {
			marker := match[1]
			switch {
			case !inFence:
				inFence, fenceChar, fenceLen = true, marker[0], len(marker)
			case marker[0] == fenceChar && len(marker) >= fenceLen:
				inFence = false
			}
			if collecting {
				versionText.WriteString(line + "\n")
			}
			continue
		}

		if !inFence {
			if level, heading, isHeading := parseHeading(line); isHeading {
				if version, isVersion := changelogVersion(heading); isVersion {
					flush()
					currentVersion, currentLevel, collecting = version, level, true
					continue
				}

				// A heading that does not announce a release either
				// belongs to the current release as a sub-heading
				// ("### Added") or, at the version's own level or
				// above, ends it: a document title ("# Changelog"),
				// an "Unreleased" section and trailing remarks are
				// not part of any release's notes.
				if collecting && level > currentLevel {
					versionText.WriteString(line + "\n")
				} else {
					flush()
				}
				continue
			}
		}

		if collecting {
			versionText.WriteString(line + "\n")
		}
	}

	flush()

	versions := make(map[string]string, len(rawVersions))
	for key, changelog := range rawVersions {
		html, err := markdown.ToHTML([]byte(changelog))
		if err != nil {
			return nil, err
		}

		versions[key] = html
	}

	return versions, nil
}

// lookupChangelogVersion finds a release in a parsed changelog, tolerating
// a leading "v" on either the requested or the written version.
func lookupChangelogVersion(changelogs map[string]string, version string) (string, bool) {
	if text, ok := changelogs[version]; ok {
		return text, true
	}
	if text, ok := changelogs[strings.TrimPrefix(version, "v")]; ok {
		return text, true
	}
	text, ok := changelogs["v"+version]

	return text, ok
}

func parseExtensionMarkdownChangelog(ext Extension) (*ExtensionChangelog, error) {
	v, err := ext.GetVersion()
	if err != nil {
		return nil, err
	}

	changelogs, err := parseMarkdownChangelogInPath(ext.GetPath())
	if err != nil {
		return nil, err
	}

	changelogEn, ok := changelogs["en-GB"]
	if !ok {
		return nil, fmt.Errorf("english changelog in version %s is missing", v.String())
	}

	changelogEnVersion, ok := lookupChangelogVersion(changelogEn, v.String())
	if !ok {
		return nil, errors.New("english changelog is missing")
	}

	changelogDe, ok := changelogs["de-DE"]
	if !ok {
		changelogDe = changelogEn
	}

	changelogDeVersion, ok := lookupChangelogVersion(changelogDe, v.String())
	if !ok {
		return nil, fmt.Errorf("german changelog in version %s is missing", v.String())
	}

	allChangelogsInVersion := make(map[string]string)

	for key, changelog := range changelogs {
		changelogVersion, ok := lookupChangelogVersion(changelog, v.String())
		if !ok {
			continue
		}

		allChangelogsInVersion[key] = changelogVersion
	}

	return &ExtensionChangelog{German: changelogDeVersion, English: changelogEnVersion, Changelogs: allChangelogsInVersion}, nil
}
