package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCI(t *testing.T) {
	// Establish a clean baseline by clearing every known CI variable. The test
	// suite itself often runs in CI, so this prevents leaking outer state.
	for _, env := range ciEnvVars {
		t.Setenv(env, "")
	}
	assert.False(t, IsCI())

	t.Setenv("CI", "true")
	assert.True(t, IsCI())

	t.Setenv("CI", "")
	t.Setenv("GITLAB_CI", "true")
	assert.True(t, IsCI())

	t.Setenv("GITLAB_CI", "")
	t.Setenv("GITHUB_ACTIONS", "true")
	assert.True(t, IsCI())
}
