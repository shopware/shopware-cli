package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadMissingFileReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed.json")

	f, err := Read(path)
	require.NoError(t, err)

	assert.Equal(t, FileVersion, f.Version)
	assert.Empty(t, f.Installed)
}

func TestReadParsesEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed.json")
	content := `{"version":1,"installed":[{"name":"deployment-helper","client":"codex","scope":"global","requestedTag":"latest","resolvedRevision":"abc123"}]}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	f, err := Read(path)
	require.NoError(t, err)

	require.Len(t, f.Installed, 1)
	assert.Equal(t, "deployment-helper", f.Installed[0].Name)
	assert.Equal(t, "codex", f.Installed[0].Client)
	assert.Equal(t, ScopeGlobal, f.Installed[0].Scope)
	assert.Equal(t, "latest", f.Installed[0].RequestedTag)
	assert.Equal(t, "abc123", f.Installed[0].ResolvedRevision)
}

func TestReadInvalidJSONErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o644))

	_, err := Read(path)
	require.Error(t, err)
}
