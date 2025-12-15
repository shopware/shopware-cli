package ci

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/logging"
)

var gitlabSectionRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// GitlabCi implements CiHelper for GitLab CI.
type GitlabCi struct{}

type GitlabCiSection struct {
	name  string
	start time.Time
}

func gitlabSectionId(name string) string {
	return gitlabSectionRegex.ReplaceAllString(strings.ToLower(name), "_")
}

// SectionStart starts a new log section.
func (g *GitlabCi) Section(ctx context.Context, name string) Section {
	sectionId := gitlabSectionId(name)
	fmt.Printf("section_start:%d:%s\r\x1b[0K%s\n", time.Now().Unix(), sectionId, name)
	return GitlabCiSection{
		name:  name,
		start: time.Now(),
	}
}

// SectionEnd ends the current log section.
func (g GitlabCiSection) End(ctx context.Context) {
	sectionId := gitlabSectionId(g.name)
	logging.FromContext(ctx).Infof("%s took %s", g.name, time.Since(g.start))
	fmt.Printf("section_end:%d:%s\r\x1b[0K\n", time.Now().Unix(), sectionId)
}
