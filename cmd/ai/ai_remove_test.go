package ai

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/install"
	"github.com/shopware/shopware-cli/internal/ai/state"
)

func TestWriteRemoveResultJSON(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeRemoveResult(&buf, formatJSON, install.RemoveResult{
		Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeProject,
		Removed: true, Command: []string{"npx", "skills", "remove"},
	}))
	assert.JSONEq(t, `{"name":"shopware-cli","agent":"claude-code","scope":"project","removed":true,"dryRun":false,"command":["npx","skills","remove"]}`, buf.String())
}

func TestWriteRemoveResultNotRecorded(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeRemoveResult(&buf, formatTable, install.RemoveResult{
		Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeGlobal, Removed: false,
	}))
	assert.Contains(t, buf.String(), "nothing to remove")
}

func TestWriteRemoveResultTable(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeRemoveResult(&buf, formatTable, install.RemoveResult{
		Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeGlobal,
		Removed: true, Command: []string{"npx", "skills", "remove", "shopware-cli"},
	}))
	assert.Contains(t, buf.String(), "Removed shopware-cli")
}
