package extension

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	goldmarkExtension "github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

func parseMarkdownChangelogInPath(path string) (map[string]map[string]string, error) {
	files, err := filepath.Glob(fmt.Sprintf("%s/CHANGELOG*.md", path))
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
			return nil, fmt.Errorf("parseMarkdownChangelogInPath: %v", err)
		}

		changelogs[language], err = parseMarkdownChangelog(string(content))

		if err != nil {
			return nil, fmt.Errorf("parseMarkdownChangelogInPath: %v", err)
		}
	}

	return changelogs, nil
}

func parseMarkdownChangelog(content string) (map[string]string, error) {
	versions := make(map[string]string)
	currentVersion := ""
	versionText := ""

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "#") {
			if len(currentVersion) > 0 && len(versionText) > 0 {
				versions[currentVersion] = versionText
			}

			currentVersion = strings.Trim(strings.TrimPrefix(line, "#"), " ")
			versionText = ""
		} else {
			versionText = strings.Trim(versionText+"\n"+line, " ")
		}
	}

	versions[currentVersion] = versionText

	for key, changelog := range versions {
		var buf bytes.Buffer

		err := GetConfiguredGoldMark().Convert([]byte(changelog), &buf)
		if err != nil {
			return nil, err
		}

		versions[key] = buf.String()
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

	changelogEnVersion, ok := changelogEn[v.String()]
	if !ok {
		return nil, fmt.Errorf("english changelog is missing")
	}

	changelogDe, ok := changelogs["de-DE"]
	if !ok {
		changelogDe = changelogEn
	}

	changelogDeVersion, ok := changelogDe[v.String()]
	if !ok {
		return nil, fmt.Errorf("german changelog in version %s is missing", v.String())
	}

	allChangelogsInVersion := make(map[string]string)

	for key, changelog := range changelogs {
		changelogVersion, ok := changelog[v.String()]
		if !ok {
			continue
		}

		allChangelogsInVersion[key] = changelogVersion
	}

	return &ExtensionChangelog{German: changelogDeVersion, English: changelogEnVersion, Changelogs: allChangelogsInVersion}, nil
}

func GetConfiguredGoldMark() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(goldmarkExtension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)
}
