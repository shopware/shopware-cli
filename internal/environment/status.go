package environment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
)

// Status is the remembered outcome of the last test of an environment.
type Status struct {
	Ready    bool      `json:"ready"`
	Limited  bool      `json:"limited,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	TestedAt time.Time `json:"tested_at"`
	// Fingerprint identifies the configuration that was tested, so an edited
	// environment counts as untested again.
	Fingerprint string `json:"fingerprint"`
}

// Store keeps test results per project config file in the user's cache
// directory. Results describe what this machine can reach, so they do not
// belong in the committed project config.
type Store struct {
	path     string
	statuses map[string]Status
}

// OpenStore loads the results remembered for the project config at configPath.
// A missing or unreadable cache file yields an empty store.
func OpenStore(configPath string) *Store {
	if abs, err := filepath.Abs(configPath); err == nil {
		configPath = abs
	}
	sum := sha256.Sum256([]byte(configPath))

	store := &Store{
		path:     filepath.Join(system.GetShopwareCliCacheDir(), "environments", hex.EncodeToString(sum[:8])+".json"),
		statuses: map[string]Status{},
	}

	if content, err := os.ReadFile(store.path); err == nil {
		if err := json.Unmarshal(content, &store.statuses); err != nil || store.statuses == nil {
			store.statuses = map[string]Status{}
		}
	}

	return store
}

// Lookup returns the last result for an environment. Tested is false when the
// environment was never tested or changed since.
func (s *Store) Lookup(name string, env *shop.EnvironmentConfig) (status Status, tested bool, changed bool) {
	status, ok := s.statuses[name]
	if !ok {
		return Status{}, false, false
	}
	if status.Fingerprint != fingerprint(env) {
		return Status{}, false, true
	}
	return status, true, false
}

// Record remembers the result of testing env.
func (s *Store) Record(env *shop.EnvironmentConfig, result Result) error {
	s.statuses[result.Name] = Status{
		Ready:       result.Ready,
		Limited:     result.Limited,
		Reason:      result.Reason,
		TestedAt:    result.TestedAt,
		Fingerprint: fingerprint(env),
	}
	return s.save()
}

// Rename moves a remembered result to the new environment name.
func (s *Store) Rename(from, to string) error {
	status, ok := s.statuses[from]
	if !ok {
		return nil
	}
	delete(s.statuses, from)
	s.statuses[to] = status
	return s.save()
}

// Forget drops the remembered result of a removed environment.
func (s *Store) Forget(name string) error {
	if _, ok := s.statuses[name]; !ok {
		return nil
	}
	delete(s.statuses, name)
	return s.save()
}

func (s *Store) save() error {
	content, err := json.MarshalIndent(s.statuses, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("cannot store environment status: %w", err)
	}
	if err := os.WriteFile(s.path, content, 0o600); err != nil {
		return fmt.Errorf("cannot store environment status: %w", err)
	}
	return nil
}

func fingerprint(env *shop.EnvironmentConfig) string {
	content, err := yaml.Marshal(env)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// Overview is one environment with its remembered status, ready for display.
type Overview struct {
	Name   string
	Type   string
	Config *shop.EnvironmentConfig
	// Ready is false for untested, changed and failed environments.
	Ready   bool
	Limited bool
	// LastTest reads "2 h ago", "not tested yet" or the failure reason.
	LastTest string
	TestedAt time.Time
}

// Describe combines an environment with its remembered status.
func (s *Store) Describe(name string, env *shop.EnvironmentConfig, now time.Time) Overview {
	overview := Overview{Name: name, Type: env.Type, Config: env}
	if overview.Type == "" {
		overview.Type = "local"
	}

	status, tested, changed := s.Lookup(name, env)
	switch {
	case changed:
		overview.LastTest = "changed, not tested yet"
	case !tested:
		overview.LastTest = "not tested yet"
	case !status.Ready:
		overview.TestedAt = status.TestedAt
		overview.LastTest = status.Reason
	default:
		overview.Ready, overview.Limited = true, status.Limited
		overview.TestedAt = status.TestedAt
		overview.LastTest = FormatAge(status.TestedAt, now)
	}

	return overview
}

// FormatAge renders how long ago t was, in the coarse steps a status list needs.
func FormatAge(t, now time.Time) string {
	age := now.Sub(t)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%d min ago", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(age.Hours()))
	case age < 48*time.Hour:
		return "yesterday"
	default:
		return fmt.Sprintf("%d days ago", int(age.Hours()/24))
	}
}
