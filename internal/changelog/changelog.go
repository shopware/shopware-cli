package changelog

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/shopware/shopware-cli/internal/git"
)

//go:embed changelog.tpl
var defaultChangelogTpl string

type Config struct {
	// Specifies whether the changelog should be generated.
	Enabled bool `yaml:"enabled"`
	// Specifies the pattern to match the commits.
	Pattern string `yaml:"pattern,omitempty"`
	// Specifies the template to use for the changelog.
	Template string `yaml:"template,omitempty"`
	// Specifies the variables to use for the changelog.
	Variables map[string]string `yaml:"variables,omitempty"`
	// Specifies the URL of the VCS repository.
	VCSURL string `yaml:"-"`
}

type Commit struct {
	Message   string
	Hash      string
	Variables map[string]string
}

// GenerateChangelog generates a changelog from the git repository.
func GenerateChangelog(ctx context.Context, repository string, cfg Config) (string, error) {
	var err error

	if cfg.Template == "" {
		cfg.Template = defaultChangelogTpl
	}

	if strings.Contains(cfg.Template, "Config.VCSURL") {
		cfg.VCSURL, err = git.GetPublicVCSURL(ctx, repository)
	}

	if err != nil {
		return "", err
	}

	commits, err := git.GetCommits(ctx, repository)
	if err != nil {
		return "", err
	}

	return renderChangelog(commits, cfg)
}

func renderChangelog(commits []git.GitCommit, cfg Config) (string, error) {
	var matcher *regexp.Regexp
	if cfg.Pattern != "" {
		matcher = regexp.MustCompile(cfg.Pattern)
	}

	variableMatchers := map[string]*regexp.Regexp{}
	for key, value := range cfg.Variables {
		variableMatchers[key] = regexp.MustCompile(value)
	}

	changelog := make([]Commit, 0)
	for _, commit := range commits {
		if matcher != nil && !matcher.MatchString(commit.Message) {
			continue
		}

		parsed := Commit{
			Message:   commit.Message,
			Hash:      commit.Hash,
			Variables: make(map[string]string),
		}

		for key, variableMatcher := range variableMatchers {
			matches := variableMatcher.FindStringSubmatch(commit.Message)
			if len(matches) > 0 {
				parsed.Variables[key] = matches[1]
			} else {
				parsed.Variables[key] = ""
			}
		}

		changelog = append(changelog, parsed)
	}

	templateParsed := template.Must(template.New("changelog").Parse(cfg.Template))

	templateContext := map[string]interface{}{
		"Commits": changelog,
		"Config":  cfg,
	}

	var buf bytes.Buffer
	if err := templateParsed.Execute(&buf, templateContext); err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	return strings.Trim(buf.String(), "\n"), nil
}
