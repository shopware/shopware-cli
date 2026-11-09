package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCIEnvironment(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "CI true", env: map[string]string{"CI": "true"}, want: true},
		{name: "GitHub Actions", env: map[string]string{"GITHUB_ACTIONS": "true"}, want: true},
		{name: "GitLab CI", env: map[string]string{"GITLAB_CI": "true"}, want: true},
		{name: "Jenkins", env: map[string]string{"JENKINS_URL": "https://jenkins.example"}, want: true},
		{name: "no CI", env: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsCIEnvironment(mapGetenv(tt.env)))
		})
	}
}

func mapGetenv(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}

func TestCIProvider(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "github", env: map[string]string{"GITHUB_ACTIONS": "true", "CI": "true"}, want: "github"},
		{name: "gitlab", env: map[string]string{"GITLAB_CI": "true", "CI": "true"}, want: "gitlab"},
		{name: "bitbucket pipelines", env: map[string]string{"BITBUCKET_BUILD_NUMBER": "7"}, want: "bitbucket"},
		{name: "bitbucket repository name alone is not CI", env: map[string]string{"BITBUCKET_REPO_FULL_NAME": "team/repo"}, want: ""},
		{name: "generic CI", env: map[string]string{"CI": "true"}, want: "other"},
		{name: "known vendor without a name", env: map[string]string{"JENKINS_URL": "https://ci.example"}, want: "other"},
		{name: "developer machine", env: map[string]string{}, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, CIProvider(mapGetenv(tc.env)))
		})
	}
}
