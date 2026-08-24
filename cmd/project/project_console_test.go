package project

import (
	"os"
	"testing"

	"github.com/mattn/go-isatty"
	"github.com/stretchr/testify/assert"
)

func TestConsoleCommandContext(t *testing.T) {
	ctx := t.Context()
	got := consoleCommandContext(ctx)

	if isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd()) {
		assert.NotEqual(t, ctx, got)
		return
	}

	assert.Equal(t, ctx, got)
}
