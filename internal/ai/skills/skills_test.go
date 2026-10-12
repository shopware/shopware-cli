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

func TestOwnerRepo(t *testing.T) {
	assert.Equal(t, "shopware/deployment-helper", OwnerRepo("https://github.com/shopware/deployment-helper"))
	assert.Equal(t, "shopware/deployment-helper", OwnerRepo("https://github.com/shopware/deployment-helper.git"))
}

func TestSource(t *testing.T) {
	// A ref pins the skill via a tree URL.
	assert.Equal(t,
		"https://github.com/shopware/deployment-helper/tree/v0.1.9/skills/deployment-helper",
		Source("https://github.com/shopware/deployment-helper", "v0.1.9", "deployment-helper"))

	// No ref falls back to the bare owner/repo (default branch).
	assert.Equal(t,
		"shopware/deployment-helper",
		Source("https://github.com/shopware/deployment-helper", "", "deployment-helper"))
}

func TestNormalizeRef(t *testing.T) {
	assert.Equal(t, "v0.1.9", NormalizeRef("0.1.9"), "bare version gets the v prefix")
	assert.Equal(t, "v1.2", NormalizeRef("1.2"), "major.minor is a version too")
	assert.Equal(t, "v0.2.0-RC1", NormalizeRef("0.2.0-RC1"), "pre-release suffix is kept")
	assert.Equal(t, "v0.1.9", NormalizeRef("v0.1.9"), "already prefixed stays")
	assert.Equal(t, "main", NormalizeRef("main"), "a branch name is left alone")
	assert.Equal(t, "2024-hotfix", NormalizeRef("2024-hotfix"), "a digit-leading branch is left alone")
	assert.Equal(t, "1a2b3c4", NormalizeRef("1a2b3c4"), "a commit sha is left alone")
	assert.Equal(t, "", NormalizeRef(""), "empty stays empty")
}
