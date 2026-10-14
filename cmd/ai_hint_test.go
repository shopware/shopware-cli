package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAIHint stubs the recommendation and isolates the suppression cache, so the
// test exercises printAIHint's own gating without invoking skills.sh.
func setupAIHint(t *testing.T) {
	t.Helper()
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir()) // MarkShown writes here
	t.Setenv("CI", "")

	prev := suggest
	suggest = func(context.Context) (string, string) {
		return "claude-code", "Shopware CLI guidance is available for claude-code.\nTo enable it, ask the user to run: shopware-cli ai add shopware-cli --agent claude-code --global"
	}
	t.Cleanup(func() { suggest = prev })
}

func TestPrintAIHintShows(t *testing.T) {
	setupAIHint(t)

	var buf bytes.Buffer
	printAIHint(t.Context(), &buf, []string{"project", "list"})

	assert.Contains(t, buf.String(), "ai add shopware-cli --agent claude-code")
}

func TestPrintAIHintSilentWhenNoSuggestion(t *testing.T) {
	setupAIHint(t)
	suggest = func(context.Context) (string, string) { return "", "" }

	var buf bytes.Buffer
	printAIHint(t.Context(), &buf, []string{"project", "list"})

	assert.Empty(t, buf.String())
}

func TestPrintAIHintSilentDuringCompletion(t *testing.T) {
	setupAIHint(t)

	var buf bytes.Buffer
	printAIHint(t.Context(), &buf, []string{"__complete", "project", "list", ""})
	assert.Empty(t, buf.String(), "shell completion must not trigger the hint")
}

func TestPrintAIHintSilentInCI(t *testing.T) {
	setupAIHint(t)
	t.Setenv("CI", "true")

	var buf bytes.Buffer
	printAIHint(t.Context(), &buf, []string{"project", "list"})

	assert.Empty(t, buf.String())
}

func TestPrintAIHintSkipsAICommand(t *testing.T) {
	setupAIHint(t)

	var buf bytes.Buffer
	printAIHint(t.Context(), &buf, []string{"ai", "list"})
	assert.Empty(t, buf.String(), "do not nag while using ai commands")
}

func TestPrintAIHintNoHintFlag(t *testing.T) {
	setupAIHint(t)
	require.NoError(t, rootCmd.PersistentFlags().Set("no-ai-hint", "true"))
	t.Cleanup(func() { _ = rootCmd.PersistentFlags().Set("no-ai-hint", "false") })

	var buf bytes.Buffer
	printAIHint(t.Context(), &buf, []string{"project", "list"})
	assert.Empty(t, buf.String(), "--no-ai-hint suppresses it")
}
