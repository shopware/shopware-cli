package ci

import "os"

// ciEnvVars are environment variables set by common CI providers. Most of them
// (GitHub Actions, GitLab CI, CircleCI, Travis, ...) set CI to a non-empty
// value, but a few well-known providers are checked explicitly as well.
var ciEnvVars = []string{
	"CI",
	"GITHUB_ACTIONS",
	"GITLAB_CI",
	"BITBUCKET_BUILD_NUMBER",
	"JENKINS_URL",
	"TEAMCITY_VERSION",
	"BUILDKITE",
	"DRONE",
}

// IsCI reports whether the process appears to be running inside a CI
// environment by checking common CI provider environment variables.
func IsCI() bool {
	for _, env := range ciEnvVars {
		if os.Getenv(env) != "" {
			return true
		}
	}

	return false
}
