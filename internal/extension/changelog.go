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

var (
	changelogVersionHeadingRegex = regexp.MustCompile(`^#{1,3}\s*\[?(?i:v)?([0-9]+\.[0-9]+(?:\.[0-9]+)*(?:-[0-9A-Za-z._-]+)?(?:\+[0-9A-Za-z._-]+)?)\]?(?:\([^)]*\)|\[[^\]]*\])?(?:\s*[-–—/:]\s*.*|\s+\([^)]*\).*|\s+\[(?i:yanked)\]|\s+[0-9]{4}[-/.][0-9]{1,2}[-/.][0-9]{1,2}.*)?$`)
	changelogUnreleasedRegex     = regexp.MustCompile(`^#{1,3}\s*\[?(?i:unreleased|unveröffentlicht)\]?(?:\([^)]*\)|\[[^\]]*\])?(?:\s+.*)?$`)
	changelogLinkRefRegex        = regexp.MustCompile(`^\s*\[[^\]]+\]:\s+\S+`)
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

func parseMarkdownChangelog(content string) (map[string]string, error) {
	versions := make(map[string]string)
	currentVersion := ""
	var versionLines []string
	var linkRefs []string

	saveCurrentVersion := func() {
		versionText := strings.TrimSpace(strings.Join(versionLines, "\n"))
		if currentVersion != "" && versionText != "" {
			versions[currentVersion] = versionText
		}
		versionLines = nil
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")

	for _, line := range strings.Split(content, "\n") {
		trimmedLine := strings.TrimSpace(line)

		if matches := changelogVersionHeadingRegex.FindStringSubmatch(trimmedLine); len(matches) > 1 {
			saveCurrentVersion()
			currentVersion = matches[1]
			continue
		}

		if changelogUnreleasedRegex.MatchString(trimmedLine) {
			saveCurrentVersion()
			currentVersion = ""
			continue
		}

		if changelogLinkRefRegex.MatchString(trimmedLine) {
			linkRefs = append(linkRefs, trimmedLine)
			continue
		}

		if currentVersion != "" {
			versionLines = append(versionLines, line)
		}
	}

	saveCurrentVersion()

	linkRefSuffix := ""
	if len(linkRefs) > 0 {
		linkRefSuffix = "\n\n" + strings.Join(linkRefs, "\n")
	}

	for key, changelog := range versions {
		html, err := markdown.ToHTML([]byte(changelog + linkRefSuffix))
		if err != nil {
			return nil, err
		}

		versions[key] = html
	}

	return versions, nil
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

	versionKey := strings.TrimPrefix(strings.TrimPrefix(v.String(), "v"), "V")

	changelogEnVersion, ok := changelogEn[versionKey]
	if !ok {
		return nil, errors.New("english changelog is missing")
	}

	changelogDe, ok := changelogs["de-DE"]
	if !ok {
		changelogDe = changelogEn
	}

	changelogDeVersion, ok := changelogDe[versionKey]
	if !ok {
		return nil, fmt.Errorf("german changelog in version %s is missing", v.String())
	}

	allChangelogsInVersion := make(map[string]string)

	for key, changelog := range changelogs {
		changelogVersion, ok := changelog[versionKey]
		if !ok {
			continue
		}

		allChangelogsInVersion[key] = changelogVersion
	}

	return &ExtensionChangelog{German: changelogDeVersion, English: changelogEnVersion, Changelogs: allChangelogsInVersion}, nil
}
