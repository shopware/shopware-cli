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
