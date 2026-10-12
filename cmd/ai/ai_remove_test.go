package ai

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/install"
	"github.com/shopware/shopware-cli/internal/ai/state"
)

func TestWriteRemoveResultNotRecorded(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeRemoveResult(&buf, install.RemoveResult{
		Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeGlobal, Removed: false,
	}))
	assert.Contains(t, buf.String(), "nothing to remove")
}

func TestWriteRemoveResultRemoved(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeRemoveResult(&buf, install.RemoveResult{
		Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeGlobal,
		Removed: true, Command: []string{"npx", "skills", "remove", "shopware-cli"},
	}))
	assert.Contains(t, buf.String(), "Removed shopware-cli")
}

func TestWriteRemoveResultDryRun(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeRemoveResult(&buf, install.RemoveResult{
		Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeGlobal,
		Removed: true, DryRun: true, Command: []string{"npx", "skills", "remove", "shopware-cli"},
	}))
	assert.Contains(t, buf.String(), "[dry-run] would remove")
}
