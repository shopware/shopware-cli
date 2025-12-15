package ci

import (
	"context"
	"os"
)

var Default CiHelper

func init() {
	Default = NewCiHelper()
}

// CiHelper is an interface for CI log formatting.
type CiHelper interface {
	Section(ctx context.Context, name string) Section
}

type Section interface {
	End(ctx context.Context)
}

// NewCiHelper returns a CiHelper for the current CI environment.
func NewCiHelper() CiHelper {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return &GithubActions{}
	}

	if os.Getenv("GITLAB_CI") == "true" {
		return &GitlabCi{}
	}

	return &DefaultCi{}
}
