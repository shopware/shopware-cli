package state

import (
	"os"
	"path/filepath"
	"testing"
)

// redirectConfigDir points the install-state location at a temp dir for the
// duration of a test and restores the seam afterwards.
func redirectConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = prev })
	return dir
}

func TestSaveCreatesDirsAndReadRoundTrips(t *testing.T) {
	base := redirectConfigDir(t)

	in := File{
		Installed: []InstalledEntry{
			{Name: "shopware-cli", Client: "claude-code", Scope: ScopeGlobal, ResolvedRevision: "0.18.3"},
		},
	}
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The file lands at the documented location and the parent dirs were created.
	want := filepath.Join(base, "shopware-cli", "ai", "installed.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected state file at %s: %v", want, err)
	}

	got, err := Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Version != FileVersion {
		t.Errorf("version = %d, want %d", got.Version, FileVersion)
	}
	if len(got.Installed) != 1 || got.Installed[0].Name != "shopware-cli" {
		t.Fatalf("round-trip mismatch: %+v", got.Installed)
	}
}

func TestSaveLeavesNoTempFileBehind(t *testing.T) {
	base := redirectConfigDir(t)

	if err := Save(File{}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(base, "shopware-cli", "ai"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "installed.json" {
			t.Errorf("unexpected leftover file: %s", e.Name())
		}
	}
}

func TestUpsertReplacesSameKeyAndAppendsNewKey(t *testing.T) {
	f := File{}

	f = Upsert(f, InstalledEntry{Name: "a", Client: "claude-code", Scope: ScopeGlobal, ResolvedRevision: "1"})
	// Same (name, client, scope) → replace, not grow.
	f = Upsert(f, InstalledEntry{Name: "a", Client: "claude-code", Scope: ScopeGlobal, ResolvedRevision: "2"})
	if len(f.Installed) != 1 {
		t.Fatalf("expected 1 entry after replace, got %d", len(f.Installed))
	}
	if f.Installed[0].ResolvedRevision != "2" {
		t.Errorf("revision not updated: %q", f.Installed[0].ResolvedRevision)
	}

	// Different client → new entry.
	f = Upsert(f, InstalledEntry{Name: "a", Client: "codex", Scope: ScopeGlobal, ResolvedRevision: "1"})
	// Different scope → new entry.
	f = Upsert(f, InstalledEntry{Name: "a", Client: "claude-code", Scope: ScopeProject, ResolvedRevision: "1"})
	if len(f.Installed) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(f.Installed), f.Installed)
	}
}
