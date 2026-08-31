package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// redirectState points the install-state path at a temp dir and returns it.
func redirectState(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	p, err := path()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))

	return p
}

func TestPath(t *testing.T) {
	redirectState(t)

	p, err := path()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(filepath.ToSlash(p), "shopware-cli/ai/installed.json"), "unexpected path: %s", p)
}

func TestReadMissingFileReturnsEmpty(t *testing.T) {
	redirectState(t)

	f, err := Read()
	require.NoError(t, err)

	assert.Equal(t, FileVersion, f.Version)
	assert.Empty(t, f.Installed)
}

func TestReadParsesEntries(t *testing.T) {
	p := redirectState(t)
	content := `{"version":1,"installed":[{"name":"deployment-helper","client":"codex","scope":"global","requestedTag":"latest","resolvedRevision":"abc123"}]}`
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))

	f, err := Read()
	require.NoError(t, err)

	require.Len(t, f.Installed, 1)
	assert.Equal(t, "deployment-helper", f.Installed[0].Name)
	assert.Equal(t, ScopeGlobal, f.Installed[0].Scope)
	assert.Equal(t, "abc123", f.Installed[0].ResolvedRevision)
}

func TestReadInvalidJSONErrors(t *testing.T) {
	p := redirectState(t)
	require.NoError(t, os.WriteFile(p, []byte("{not json"), 0o644))

	_, err := Read()
	require.Error(t, err)
}

func TestReadUnsupportedVersionErrors(t *testing.T) {
	p := redirectState(t)
	require.NoError(t, os.WriteFile(p, []byte(`{"version":2,"installed":[]}`), 0o644))

	_, err := Read()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported ai install-state version 2")
}

func TestReadMissingVersionErrors(t *testing.T) {
	p := redirectState(t)
	require.NoError(t, os.WriteFile(p, []byte(`{"installed":[]}`), 0o644))

	_, err := Read()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported ai install-state version 0")
}
