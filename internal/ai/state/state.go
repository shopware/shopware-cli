// Package state records which AI integrations the CLI has installed so that
// `ai list --installed` can report them. This package defines the on-disk file
// format and the read path; writing the file is added by #1337. Until then the
// file does not exist and Read returns an empty state.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

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
// `ai info`. #1337 fills these when it installs a skill.
type InstalledEntry struct {
	Name             string `json:"name"`
	Client           string `json:"client"`
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
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "shopware-cli", "ai", "installed.json"), nil
}

// Read loads the install-state file. A missing file is not an error: it returns
// an empty state, which is the expected situation until #1337 writes the file.
func Read() (File, error) {
	p, err := path()
	if err != nil {
		return File{}, err
	}

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
