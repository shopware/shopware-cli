package extension

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/shopware/shopware-cli/internal/markdown"
)

var (
	changelogVersionHeadingRegex = regexp.MustCompile(`^\[?(?i:v)?([0-9]+\.[0-9]+(?:\.[0-9]+)*(?:-[0-9A-Za-z._-]+)?(?:\+[0-9A-Za-z._-]+)?)\]?(?:\([^)]*\)|\[[^\]]*\])?(?:\s*[-–—/:]\s*.*|\s+\([^)]*\).*|\s+\[(?i:yanked)\]|\s+[0-9]{4}[-/.][0-9]{1,2}[-/.][0-9]{1,2}.*)?$`)
	changelogUnreleasedRegex     = regexp.MustCompile(`^\[?(?i:unreleased|unveröffentlicht)\]?(?:\([^)]*\)|\[[^\]]*\])?(?:\s+.*)?$`)
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
	source := []byte(content)
	md := markdown.New()
	document := md.Parser().Parse(text.NewReader(source))
	versions := make(map[string]string)
	currentVersion := ""
	var body bytes.Buffer

	saveCurrentVersion := func() {
		if currentVersion != "" && body.Len() > 0 {
			versions[currentVersion] = body.String()
		}
		body.Reset()
	}

	for node := document.FirstChild(); node != nil; node = node.NextSibling() {
		if heading, ok := node.(*ast.Heading); ok && heading.Level <= 3 {
			label := strings.TrimSpace(string(util.UnescapePunctuations(heading.Lines().Value(source))))
			if matches := changelogVersionHeadingRegex.FindStringSubmatch(label); len(matches) > 1 {
				saveCurrentVersion()
				currentVersion = matches[1]
				continue
			}
			if changelogUnreleasedRegex.MatchString(label) {
				saveCurrentVersion()
				currentVersion = ""
				continue
			}
		}

		if currentVersion != "" {
			if err := md.Renderer().Render(&body, source, node); err != nil {
				return nil, err
			}
		}
	}

	saveCurrentVersion()

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
