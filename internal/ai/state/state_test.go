package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	p, err := Path()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(filepath.ToSlash(p), "shopware-cli/ai/installed.json"), "unexpected path: %s", p)
}

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

func TestReadUnsupportedVersionErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":2,"installed":[]}`), 0o644))

	_, err := Read(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported ai install-state version 2")
}

func TestReadMissingVersionErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"installed":[]}`), 0o644))

	_, err := Read(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported ai install-state version 0")
}
