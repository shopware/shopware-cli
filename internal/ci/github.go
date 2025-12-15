package ci

import (
	"context"
	"fmt"
	"time"

	"github.com/shopware/shopware-cli/logging"
)

// GithubActions implements CiHelper for GitHub Actions.
type GithubActions struct{}

type GithubActionsSection struct {
	name  string
	start time.Time
}

// SectionStart starts a new log section.
func (g *GithubActions) Section(ctx context.Context, name string) Section {
	fmt.Printf("::group::%s\n", name)
	return GithubActionsSection{
		name:  name,
		start: time.Now(),
	}
}

func (s GithubActionsSection) End(ctx context.Context) {
	duration := time.Since(s.start)
	logging.FromContext(ctx).Infof("%s took %s", s.name, duration)
	fmt.Printf("::endgroup::\n")
}
