package proxy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
)

const settingsFile = "settings.json"

// LoadSettings reads the persisted proxy settings, falling back to defaults
// when the proxy has never been configured.
func LoadSettings(dir string) (Settings, error) {
	content, err := os.ReadFile(filepath.Join(dir, settingsFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultSettings(), nil
		}

		return Settings{}, err
	}

	settings := DefaultSettings()
	if err := json.Unmarshal(content, &settings); err != nil {
		return Settings{}, err
	}

	return settings, nil
}

// SaveSettings persists the proxy settings.
func SaveSettings(dir string, settings Settings) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, settingsFile), append(content, '\n'), 0o600)
}

// RegisterHost records a hostname so certificate regeneration covers it. It
// returns true when the host was not known before.
func (s *Settings) RegisterHost(host string) bool {
	if host == s.Domain || matchesWildcard(host, s.Domain) || slices.Contains(s.Hosts, host) {
		return false
	}

	s.Hosts = append(s.Hosts, host)

	return true
}
