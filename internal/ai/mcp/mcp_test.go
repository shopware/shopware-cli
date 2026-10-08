package mcp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddArgs(t *testing.T) {
	got := strings.Join(AddArgs("https://meteor.shopware.com/mcp", "meteor", "claude-code", "http", true, true), " ")
	want := "npx --yes add-mcp@2 https://meteor.shopware.com/mcp --name meteor --agent claude-code --transport http --global -y"
	assert.Equal(t, want, got)

	got = strings.Join(AddArgs("https://x/mcp", "x", "codex", "http", false, false), " ")
	assert.NotContains(t, got, "--global")
	assert.NotContains(t, got, " -y")
}

func TestRemoveArgs(t *testing.T) {
	got := strings.Join(RemoveArgs("meteor", "claude-code", true), " ")
	want := "npx --yes add-mcp@2 remove meteor --agent claude-code --global -y"
	assert.Equal(t, want, got)

	got = strings.Join(RemoveArgs("meteor", "claude-code", false), " ")
	assert.NotContains(t, got, "--global")
	assert.Contains(t, got, " -y")
}
