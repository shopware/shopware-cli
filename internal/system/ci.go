package system

import "strings"

func IsCIEnvironment(getenv func(string) string) bool {
	if strings.EqualFold(getenv("CI"), "true") {
		return true
	}

	ciEnvVars := []string{
		"GITHUB_ACTIONS",
		"GITLAB_CI",
		"JENKINS_URL",
		"BUILDKITE",
		"CIRCLECI",
		"DRONE",
		"TEAMCITY_VERSION",
		"TF_BUILD",
	}

	for _, envVar := range ciEnvVars {
		if getenv(envVar) != "" {
			return true
		}
	}

	return false
}

// CIProvider names the CI system of a run: github, gitlab, bitbucket, other, or "" outside CI.
func CIProvider(getenv func(string) string) string {
	switch {
	case getenv("GITHUB_ACTIONS") != "":
		return "github"
	case getenv("GITLAB_CI") != "":
		return "gitlab"
	case getenv("BITBUCKET_BUILD_NUMBER") != "":
		return "bitbucket"
	case IsCIEnvironment(getenv):
		return "other"
	default:
		return ""
	}
}
