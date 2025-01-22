package changelog

import (
	"os"
	"testing"

	"github.com/shopware/shopware-cli/internal/git"
	"github.com/stretchr/testify/assert"
)

func TestGenerateWithoutConfig(t *testing.T) {
	commits := []git.GitCommit{
		{
			Message: "feat: add new feature",
			Hash:    "1234567890",
		},
	}

	changelog, err := renderChangelog(commits, Config{
		VCSURL:   "https://github.com/FriendsOfShopware/FroshTools/commit",
		Template: defaultChangelogTpl,
	})

	assert.NoError(t, err)

	assert.Equal(t, "- [feat: add new feature](https://github.com/FriendsOfShopware/FroshTools/commit/1234567890)", changelog)
}

func TestTicketParsing(t *testing.T) {
	commits := []git.GitCommit{
		{
			Message: "NEXT-1234 - Fooo",
			Hash:    "1234567890",
		},
	}

	cfg := Config{
		Variables: map[string]string{
			"ticket": "^(NEXT-[0-9]+)",
		},
		Template: "{{range .Commits}}- [{{ .Message }}](https://issues.shopware.com/issues/{{ .Variables.ticket }}){{end}}",
	}

	changelog, err := renderChangelog(commits, cfg)

	assert.NoError(t, err)
	assert.Equal(t, "- [NEXT-1234 - Fooo](https://issues.shopware.com/issues/NEXT-1234)", changelog)
}

func TestIncludeFilters(t *testing.T) {
	commits := []git.GitCommit{
		{
			Message: "NEXT-1234 - Fooo",
			Hash:    "1234567890",
		},
		{
			Message: "merge foo",
			Hash:    "1234567890",
		},
	}

	cfg := Config{
		Pattern:  "^(NEXT-[0-9]+)",
		Template: defaultChangelogTpl,
	}

	changelog, err := renderChangelog(commits, cfg)

	assert.NoError(t, err)
	assert.Equal(t, "- [NEXT-1234 - Fooo](/1234567890)", changelog)
}

func TestLetAiGenerateText(t *testing.T) {
	if os.Getenv("OPENAI_TOKEN") == "" {
		t.Skip("Need OPENAI_TOKEN env")
	}

	commits := []git.GitCommit{
		{
			Message: "fix: task checker interval compare minutes instead of months",
			Hash:    "1234567890",
		},
		{
			Message: "fix: correct detection of delayed scheduled tasks (#197)",
			Hash:    "1234567890",
		},
		{
			Message: "feature: read messenger stats from transports",
			Hash:    "123",
		},
	}

	cfg := Config{
		AiEnabled: true,
		Template:  defaultChangelogTpl,
	}

	changelog, err := renderChangelog(commits, cfg)

	assert.NoError(t, err)
	assert.Contains(t, changelog, "Commits:")
}
