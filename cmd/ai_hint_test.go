package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/system"
)

// setupAIHint isolates state, project root and the suppression cache, and marks
// the process as invoked by Claude Code.
func setupAIHint(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("PROJECT_ROOT", t.TempDir())
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	t.Setenv("AI_AGENT", "claude-code_2-1-289_agent")
	t.Setenv("CI", "")
}

func TestPrintAIHintShows(t *testing.T) {
	setupAIHint(t)
	ctx := system.WithInteraction(t.Context(), true)

	var buf bytes.Buffer
	printAIHint(ctx, &buf, []string{"project", "list"})

	assert.Contains(t, buf.String(), "ai add shopware-cli --agent claude-code")
}

func TestPrintAIHintSilentWhenNonInteractive(t *testing.T) {
	setupAIHint(t)
	ctx := system.WithInteraction(t.Context(), false)

	var buf bytes.Buffer
	printAIHint(ctx, &buf, []string{"project", "list"})

	assert.Empty(t, buf.String())
}

func TestPrintAIHintSilentInCI(t *testing.T) {
	setupAIHint(t)
	t.Setenv("CI", "true")
	ctx := system.WithInteraction(t.Context(), true)

	var buf bytes.Buffer
	printAIHint(ctx, &buf, []string{"project", "list"})

	assert.Empty(t, buf.String())
}

func TestPrintAIHintSkipsAICommandAndFlag(t *testing.T) {
	setupAIHint(t)
	ctx := system.WithInteraction(t.Context(), true)

	var buf bytes.Buffer
	printAIHint(ctx, &buf, []string{"ai", "list"})
	assert.Empty(t, buf.String(), "do not nag while using ai commands")

	printAIHint(ctx, &buf, []string{"project", "list", "--no-ai-hint"})
	assert.Empty(t, buf.String(), "--no-ai-hint suppresses it")
}
