package ai

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/install"
)

func TestWriteRemoveResult(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeRemoveResult(&buf, install.RemoveResult{
		Name: "shopware-cli", Agent: "claude-code", Scope: install.ScopeProject,
		Command: []string{"npx", "skills", "remove", "shopware-cli"},
	}))
	assert.Contains(t, buf.String(), "Removed shopware-cli for claude-code (project)")
}

func TestWriteRemoveResultDryRun(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeRemoveResult(&buf, install.RemoveResult{
		Name: "shopware-cli", Agent: "claude-code", Scope: install.ScopeGlobal,
		DryRun: true, Command: []string{"npx", "skills", "remove", "shopware-cli"},
	}))
	assert.Contains(t, buf.String(), "[dry-run] would remove")
}
