package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// redirectConfigDir points the install-state location at a temp dir (via the
// userConfigDir seam) for the duration of a test and returns that base dir.
func redirectConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = prev })

	return dir
}

// statePath returns the global state file path under base, creating its directory
// so a test can write a fixture there.
func statePath(t *testing.T, base string) string {
	t.Helper()
	p := filepath.Join(base, "shopware-cli", "ai", "installed.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))

	return p
}

// --- read path ---

func TestPath(t *testing.T) {
	redirectConfigDir(t)

	p, err := path()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(filepath.ToSlash(p), "shopware-cli/ai/installed.json"), "unexpected path: %s", p)
}

func TestReadMissingFileReturnsEmpty(t *testing.T) {
	redirectConfigDir(t)

	f, err := Read()
	require.NoError(t, err)

	assert.Equal(t, FileVersion, f.Version)
	assert.Empty(t, f.Installed)
}

func TestReadParsesEntries(t *testing.T) {
	p := statePath(t, redirectConfigDir(t))
	content := `{"version":1,"installed":[{"name":"deployment-helper","agent":"codex","scope":"global","requestedTag":"latest","resolvedRevision":"abc123"}]}`
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))

	f, err := Read()
	require.NoError(t, err)

	require.Len(t, f.Installed, 1)
	assert.Equal(t, "deployment-helper", f.Installed[0].Name)
	assert.Equal(t, "codex", f.Installed[0].Agent)
	assert.Equal(t, ScopeGlobal, f.Installed[0].Scope)
	assert.Equal(t, "abc123", f.Installed[0].ResolvedRevision)
}

func TestReadInvalidJSONErrors(t *testing.T) {
	p := statePath(t, redirectConfigDir(t))
	require.NoError(t, os.WriteFile(p, []byte("{not json"), 0o644))

	_, err := Read()
	require.Error(t, err)
}

func TestReadUnsupportedVersionErrors(t *testing.T) {
	p := statePath(t, redirectConfigDir(t))
	require.NoError(t, os.WriteFile(p, []byte(`{"version":2,"installed":[]}`), 0o644))

	_, err := Read()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported ai install-state version 2")
}

func TestReadMissingVersionErrors(t *testing.T) {
	p := statePath(t, redirectConfigDir(t))
	require.NoError(t, os.WriteFile(p, []byte(`{"installed":[]}`), 0o644))

	_, err := Read()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported ai install-state version 0")
}

// --- write path ---

func TestSaveCreatesDirsAndReadRoundTrips(t *testing.T) {
	base := redirectConfigDir(t)

	in := File{
		Installed: []InstalledEntry{
			{Name: "shopware-cli", Agent: "claude-code", Scope: ScopeGlobal, ResolvedRevision: "0.18.3"},
		},
	}
	require.NoError(t, Save(in))

	// The file lands at the documented location and the parent dirs were created.
	require.FileExists(t, filepath.Join(base, "shopware-cli", "ai", "installed.json"))

	got, err := Read()
	require.NoError(t, err)
	assert.Equal(t, FileVersion, got.Version)
	require.Len(t, got.Installed, 1)
	assert.Equal(t, "shopware-cli", got.Installed[0].Name)
}

func TestSaveLeavesNoTempFileBehind(t *testing.T) {
	base := redirectConfigDir(t)

	require.NoError(t, Save(File{}))

	entries, err := os.ReadDir(filepath.Join(base, "shopware-cli", "ai"))
	require.NoError(t, err)
	for _, e := range entries {
		assert.Equal(t, "installed.json", e.Name(), "unexpected leftover file")
	}
}

func TestUpsertReplacesSameKeyAndAppendsNewKey(t *testing.T) {
	f := File{}

	f = Upsert(f, InstalledEntry{Name: "a", Agent: "claude-code", Scope: ScopeGlobal, ResolvedRevision: "1"})
	// Same (name, agent, scope) → replace, not grow.
	f = Upsert(f, InstalledEntry{Name: "a", Agent: "claude-code", Scope: ScopeGlobal, ResolvedRevision: "2"})
	require.Len(t, f.Installed, 1, "same key should replace, not append")
	assert.Equal(t, "2", f.Installed[0].ResolvedRevision)

	// Different agent → new entry.
	f = Upsert(f, InstalledEntry{Name: "a", Agent: "codex", Scope: ScopeGlobal, ResolvedRevision: "1"})
	// Different scope → new entry.
	f = Upsert(f, InstalledEntry{Name: "a", Agent: "claude-code", Scope: ScopeProject, ResolvedRevision: "1"})
	assert.Len(t, f.Installed, 3)
}

func TestRemove(t *testing.T) {
	f := File{Installed: []InstalledEntry{
		{Name: "a", Agent: "claude-code", Scope: ScopeGlobal},
		{Name: "b", Agent: "claude-code", Scope: ScopeGlobal},
	}}

	f, ok := Remove(f, "a", "claude-code", ScopeGlobal)
	require.True(t, ok, "expected removal to report true")
	require.Len(t, f.Installed, 1)
	assert.Equal(t, "b", f.Installed[0].Name)

	// A matching name+agent but different scope is not removed.
	_, ok = Remove(f, "b", "claude-code", ScopeProject)
	assert.False(t, ok, "expected no removal for a mismatched scope")

	// A missing entry reports false.
	_, ok = Remove(f, "missing", "claude-code", ScopeGlobal)
	assert.False(t, ok, "expected no removal for a missing entry")
}

func TestClearRemovesFileAndPrunesDirs(t *testing.T) {
	base := redirectConfigDir(t)

	require.NoError(t, Save(File{Installed: []InstalledEntry{{Name: "a", Agent: "c", Scope: ScopeGlobal}}}))
	require.FileExists(t, filepath.Join(base, "shopware-cli", "ai", "installed.json"))

	require.NoError(t, Clear())
	assert.NoFileExists(t, filepath.Join(base, "shopware-cli", "ai", "installed.json"))
	assert.NoDirExists(t, filepath.Join(base, "shopware-cli", "ai"))

	// Clearing an already-missing file is not an error.
	require.NoError(t, Clear())
}

func TestClearProjectRemovesFile(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, SaveProject(root, File{Installed: []InstalledEntry{{Name: "a", Agent: "c", Scope: ScopeProject}}}))
	require.NoError(t, ClearProject(root))
	assert.NoFileExists(t, filepath.Join(root, ".shopware-cli", "ai", "installed.json"))
}

func TestSaveProjectRoundTrip(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, SaveProject(root, File{Installed: []InstalledEntry{
		{Name: "deployment-helper", Agent: "claude-code", Scope: ScopeProject, ResolvedRevision: "v1.2.0"},
	}}))

	require.FileExists(t, filepath.Join(root, ".shopware-cli", "ai", "installed.json"))

	got, err := ReadProject(root)
	require.NoError(t, err)
	require.Len(t, got.Installed, 1)
	assert.Equal(t, "deployment-helper", got.Installed[0].Name)
}

func TestGlobalAndProjectStateAreIndependent(t *testing.T) {
	redirectConfigDir(t) // global state → temp
	root := t.TempDir()  // project state

	require.NoError(t, Save(File{Installed: []InstalledEntry{{Name: "g", Agent: "c", Scope: ScopeGlobal}}}))
	require.NoError(t, SaveProject(root, File{Installed: []InstalledEntry{{Name: "p", Agent: "c", Scope: ScopeProject}}}))

	g, err := Read()
	require.NoError(t, err)
	require.Len(t, g.Installed, 1)
	assert.Equal(t, "g", g.Installed[0].Name, "global state leaked")

	p, err := ReadProject(root)
	require.NoError(t, err)
	require.Len(t, p.Installed, 1)
	assert.Equal(t, "p", p.Installed[0].Name, "project state leaked")

	// A directory with no project state reads as empty.
	empty, err := ReadProject(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, empty.Installed)
}
