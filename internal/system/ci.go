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
