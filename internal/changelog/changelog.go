package changelog

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/sashabaranov/go-openai"

	"github.com/shopware/shopware-cli/internal/git"
)

//go:embed changelog.tpl
var defaultChangelogTpl string

type Config struct {
	Enabled   bool              `yaml:"enabled"`
	Pattern   string            `yaml:"pattern,omitempty"`
	Template  string            `yaml:"template,omitempty"`
	Variables map[string]string `yaml:"variables,omitempty"`
	AiEnabled bool              `yaml:"ai_enabled,omitempty"`
	VCSURL    string            `yaml:"-"`
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

	aiMessage, err := generateAiMessage(changelog, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to generate AI message: %v", err)
	}

	templateContext := map[string]interface{}{
		"Commits":     changelog,
		"Config":      cfg,
		"AiSummarize": aiMessage,
	}

	var buf bytes.Buffer
	if err := templateParsed.Execute(&buf, templateContext); err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	return strings.Trim(buf.String(), "\n"), nil
}

func generateAiMessage(changelog []Commit, cfg Config) (string, error) {
	if !cfg.AiEnabled {
		return "", nil
	}

	aiRequestBody := ""

	for _, commit := range changelog {
		aiRequestBody += commit.Message + "\n"
	}

	aiRequestBody += "Please summarize the changelog into 1-2 sentences and ignore chore or build things"

	client := openai.NewClient(os.Getenv("OPENAI_TOKEN"))
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:  openai.GPT3Dot5Turbo,
			Stream: false,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: aiRequestBody,
				},
			},
		},
	)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) > 0 {
		return resp.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("got no response from openai: %w", err)
}
