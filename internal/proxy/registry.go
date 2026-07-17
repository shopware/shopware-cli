package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ProjectEntry is one project registered with the shared proxy.
type ProjectEntry struct {
	// ProjectRoot is the canonical (symlink-resolved) absolute path to the
	// project directory. It is the registry's key.
	ProjectRoot  string    `json:"project_root"`
	Hostname     string    `json:"hostname"`
	RegisteredAt time.Time `json:"registered_at"`
	// PreviousAppURL is the APP_URL the project had before registration, so
	// "proxy down" can restore it (empty means the 127.0.0.1:8000 default).
	PreviousAppURL string `json:"previous_app_url,omitempty"`
	// PreviousConfig captures the url keys of .shopware-project.yml before
	// registration rewrote them; nil means the file was not touched.
	PreviousConfig *ConfigURLState `json:"previous_config,omitempty"`
}

// Registry is the local record of projects registered with the shared proxy.
type Registry struct {
	Projects []ProjectEntry `json:"projects"`
}

const registryFileName = "registry.json"

// LoadRegistry reads the project registry from the shared state directory. A
// missing file is not an error; it returns an empty registry.
func LoadRegistry() (Registry, error) {
	dir, err := StateDir()
	if err != nil {
		return Registry{}, err
	}

	data, err := os.ReadFile(filepath.Join(dir, registryFileName))
	if os.IsNotExist(err) {
		return Registry{}, nil
	}
	if err != nil {
		return Registry{}, err
	}

	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return Registry{}, err
	}

	return reg, nil
}

// Upsert adds entry, replacing any existing entry with the same ProjectRoot.
func (r *Registry) Upsert(entry ProjectEntry) {
	for i, e := range r.Projects {
		if e.ProjectRoot == entry.ProjectRoot {
			r.Projects[i] = entry
			return
		}
	}

	r.Projects = append(r.Projects, entry)
}

// Remove deletes the entry for projectRoot, reporting whether one was removed.
func (r *Registry) Remove(projectRoot string) bool {
	for i, e := range r.Projects {
		if e.ProjectRoot == projectRoot {
			r.Projects = append(r.Projects[:i], r.Projects[i+1:]...)
			return true
		}
	}

	return false
}

// Find returns the entry for projectRoot, if any.
func (r Registry) Find(projectRoot string) (ProjectEntry, bool) {
	for _, e := range r.Projects {
		if e.ProjectRoot == projectRoot {
			return e, true
		}
	}

	return ProjectEntry{}, false
}

// FindByHostname returns the entry using hostname, if any, ignoring the
// entry (if any) belonging to exceptProjectRoot. It is used to detect
// hostname collisions between two different projects.
func (r Registry) FindByHostname(hostname, exceptProjectRoot string) (ProjectEntry, bool) {
	for _, e := range r.Projects {
		if e.Hostname == hostname && e.ProjectRoot != exceptProjectRoot {
			return e, true
		}
	}

	return ProjectEntry{}, false
}

// Save writes the registry back to the shared state directory.
func (r Registry) Save() error {
	dir, err := StateDir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, registryFileName), data, 0o600)
}
