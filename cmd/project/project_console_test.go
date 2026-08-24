package project

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/mattn/go-isatty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestShouldRunComposerScript(t *testing.T) {
	scripts := []shop.ComposerScript{
		{Name: "phpstan", Aliases: []string{"stan"}},
		{Name: "list"},
	}

	var console shop.ConsoleResponse
	require.NoError(t, json.Unmarshal([]byte(`{"commands":[{"name":"cache:clear"}]}`), &console))

	assert.True(t, shouldRunComposerScript("phpstan", &console, scripts))
	assert.True(t, shouldRunComposerScript("stan", &console, scripts))
	assert.False(t, shouldRunComposerScript("list", &console, scripts))
	assert.False(t, shouldRunComposerScript("help", &console, scripts))
	assert.False(t, shouldRunComposerScript("completion", &console, scripts))
	assert.False(t, shouldRunComposerScript("composer", &console, scripts))
	assert.False(t, shouldRunComposerScript("cache:clear", &console, scripts))
	assert.False(t, shouldRunComposerScript("missing", &console, scripts))
	assert.True(t, shouldRunComposerScript("phpstan", nil, scripts))
}

func TestIsComposerProxy(t *testing.T) {
	assert.False(t, isComposerProxy(nil))
	assert.False(t, isComposerProxy([]string{"cache:clear"}))
	assert.True(t, isComposerProxy([]string{"composer"}))
	assert.True(t, isComposerProxy([]string{"composer", "install", "--no-dev"}))
}

func TestComposerScriptArgs(t *testing.T) {
	assert.Equal(t, []string{"run-script", "--timeout=0", "--", "phpstan"}, composerScriptArgs("phpstan", nil))
	assert.Equal(t, []string{"run-script", "--timeout=0", "--", "phpstan", "--", "--memory-limit=2G"}, composerScriptArgs("phpstan", []string{"--", "--memory-limit=2G"}))
}

func TestFormatComposerScriptsList(t *testing.T) {
	assert.Empty(t, formatComposerScriptsList(nil))

	out := formatComposerScriptsList([]shop.ComposerScript{
		{Name: "ecs"},
		{Name: "phpstan", Description: "Run PHPStan"},
	})

	assert.Contains(t, out, "\n composer\n")
	assert.Contains(t, out, "  ecs\n")
	assert.Contains(t, out, "phpstan")
	assert.Contains(t, out, "Run PHPStan")
}

func TestCompletionWithDescription(t *testing.T) {
	assert.Equal(t, "ecs", completionWithDescription("ecs", ""))
	assert.Equal(t, "phpstan\tRun PHPStan", completionWithDescription("phpstan", "Run PHPStan"))
}

func TestCommandListCompletions(t *testing.T) {
	var resp shop.ConsoleResponse
	require.NoError(t, json.Unmarshal([]byte(`{
		"commands": [
			{"name": "install", "description": "Installs the project dependencies"},
			{"name": "hidden:cmd", "hidden": true}
		]
	}`), &resp))

	assert.Equal(t, []string{"install\tInstalls the project dependencies"}, commandListCompletions(&resp))
	assert.Nil(t, commandListCompletions(nil))
}

func TestFilterUsedCompletions(t *testing.T) {
	assert.Equal(t, []string{"--no-dev"}, filterUsedCompletions([]string{"--no-dev", "--dry-run"}, []string{"install", "--dry-run"}))
}

func TestConsoleCommandContext(t *testing.T) {
	ctx := t.Context()
	got := consoleCommandContext(ctx)

	if isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd()) {
		assert.NotEqual(t, ctx, got)
		return
	}

	assert.Equal(t, ctx, got)
}
