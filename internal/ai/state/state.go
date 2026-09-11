// Package state records which AI integrations the CLI has installed so that
// `ai list --installed` can report them. It defines the on-disk file format,
// the read path, and the write path used by `ai add` / `ai remove`.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// userConfigDir is a seam over os.UserConfigDir so tests can redirect the
// install-state location without touching the real user configuration.
var userConfigDir = os.UserConfigDir

// FileVersion is the install-state file format version. It is a major-only
// integer, like the manifest: bump it only on a breaking change to the file
// shape.
const FileVersion = 1

// Scope is where an integration is installed.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

// InstalledEntry records a single CLI-managed installation. The field names are
// a public contract (camelCase), reported by `ai list --installed` and
// `ai info`.
type InstalledEntry struct {
	Name             string `json:"name"`
	Agent            string `json:"agent"`
	Scope            Scope  `json:"scope"`
	RequestedTag     string `json:"requestedTag"`
	ResolvedRevision string `json:"resolvedRevision"`
}

// File is the on-disk install-state document.
type File struct {
	Version   int              `json:"version"`
	Installed []InstalledEntry `json:"installed"`
}

// path is the global install-state file location
// ($UserConfigDir/shopware-cli/ai/installed.json).
func path() (string, error) {
	configDir, err := userConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "shopware-cli", "ai", "installed.json"), nil
}

// projectPath is the project-scoped install-state location
// (<projectRoot>/.shopware-cli/ai/installed.json). Project-scoped installs live
// with the project, mirroring where skills.sh writes the agent config.
func projectPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".shopware-cli", "ai", "installed.json")
}

// Read loads the global install-state file. A missing file is not an error: it
// returns an empty state.
func Read() (File, error) {
	p, err := path()
	if err != nil {
		return File{}, err
	}

	return readFrom(p)
}

// ReadProject loads the project-scoped install-state file under projectRoot.
func ReadProject(projectRoot string) (File, error) {
	return readFrom(projectPath(projectRoot))
}

// readFrom loads and validates an install-state file. A missing file yields an
// empty state rather than an error.
func readFrom(p string) (File, error) {
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return File{Version: FileVersion}, nil
	}
	if err != nil {
		return File{}, err
	}

	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, fmt.Errorf("parse ai install-state %s: %w", p, err)
	}
	if f.Version != FileVersion {
		return File{}, fmt.Errorf("unsupported ai install-state version %d (expected %d)", f.Version, FileVersion)
	}

	return f, nil
}

// Upsert returns f with e added, or with the existing entry that has the same
// (name, agent, scope) replaced. The same integration can be installed for
// different agents and scopes, so all three fields form the identity. This
// makes a repeated `ai add` idempotent: the list does not grow.
func Upsert(f File, e InstalledEntry) File {
	for i := range f.Installed {
		x := f.Installed[i]
		if x.Name == e.Name && x.Agent == e.Agent && x.Scope == e.Scope {
			f.Installed[i] = e
			return f
		}
	}

	f.Installed = append(f.Installed, e)

	return f
}

// Remove returns f with the entry matching (name, agent, scope) dropped, and
// whether an entry was removed. `ai remove` uses this to drop only what the CLI
// recorded.
func Remove(f File, name, agent string, scope Scope) (File, bool) {
	for i := range f.Installed {
		x := f.Installed[i]
		if x.Name == name && x.Agent == agent && x.Scope == scope {
			f.Installed = append(f.Installed[:i], f.Installed[i+1:]...)
			return f, true
		}
	}

	return f, false
}

// Save writes the global install-state file atomically.
func Save(f File) error {
	p, err := path()
	if err != nil {
		return err
	}

	return saveTo(p, f)
}

// SaveProject writes the project-scoped install-state file under projectRoot.
func SaveProject(projectRoot string, f File) error {
	return saveTo(projectPath(projectRoot), f)
}

// saveTo writes an install-state file atomically: it writes a temporary file in
// the target directory and renames it into place, so a crash mid-write never
// leaves a partial file. The parent directories are created as needed.
func saveTo(p string, f File) error {
	f.Version = FileVersion
	if f.Installed == nil {
		f.Installed = []InstalledEntry{}
	}

	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "installed-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we return before the rename succeeds.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, p)
}
