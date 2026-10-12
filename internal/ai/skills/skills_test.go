package skills

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddArgs(t *testing.T) {
	got := strings.Join(AddArgs("shopware/deployment-helper", "deployment-helper", "claude-code", true, true), " ")
	want := "npx --yes skills@1 add shopware/deployment-helper --skill deployment-helper --agent claude-code --global -y"
	assert.Equal(t, want, got)

	got = strings.Join(AddArgs("src", "s", "claude-code", false, false), " ")
	assert.NotContains(t, got, "--global")
	assert.NotContains(t, got, " -y")
	assert.Contains(t, got, "--skill s")
}

func TestRemoveArgs(t *testing.T) {
	got := strings.Join(RemoveArgs("deployment-helper", "claude-code", true), " ")
	assert.Equal(t, "npx --yes skills@1 remove deployment-helper --agent claude-code --global -y", got)

	got = strings.Join(RemoveArgs("deployment-helper", "claude-code", false), " ")
	assert.NotContains(t, got, "--global")
	assert.Contains(t, got, " -y")
}

func TestSource(t *testing.T) {
	// A ref pins the skill via a tree URL.
	assert.Equal(t,
		"https://github.com/shopware/deployment-helper/tree/v0.1.9/skills/deployment-helper",
		Source("shopware/deployment-helper", "v0.1.9", "deployment-helper"))

	// No ref falls back to the bare owner/repo (default branch).
	assert.Equal(t,
		"shopware/deployment-helper",
		Source("shopware/deployment-helper", "", "deployment-helper"))
}
