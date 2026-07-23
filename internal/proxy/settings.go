package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const settingsFileName = "settings.json"

// Settings are the machine-wide proxy settings, chosen once via
// "project proxy setup" and read by every other proxy command.
type Settings struct {
	// Domain is the base domain projects are routed under. Empty means
	// DefaultDomain.
	Domain string `json:"domain,omitempty"`
}

// BaseDomain returns the configured domain, falling back to DefaultDomain.
func (s Settings) BaseDomain() string {
	if s.Domain == "" {
		return DefaultDomain
	}

	return s.Domain
}

// LoadSettings reads the machine-wide proxy settings; a missing file yields
// the defaults.
func LoadSettings() (Settings, error) {
	dir, err := StateDir()
	if err != nil {
		return Settings{}, err
	}

	data, err := os.ReadFile(filepath.Join(dir, settingsFileName))
	if os.IsNotExist(err) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, err
	}

	return settings, nil
}

// SaveSettings persists the machine-wide proxy settings.
func SaveSettings(settings Settings) error {
	dir, err := StateDir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, settingsFileName), data, 0o600)
}

var dnsLabelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// ValidateDomain checks that domain is a plain lowercase DNS name usable as
// the proxy's base domain (e.g. "shopware.local" or "dev.internal").
func ValidateDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("domain must not be empty")
	}

	if strings.Contains(domain, "://") || strings.Contains(domain, "/") {
		return fmt.Errorf("%q must be a plain domain without scheme or path", domain)
	}

	if len(domain) > 253 {
		return fmt.Errorf("%q is longer than 253 characters", domain)
	}

	if domain != strings.ToLower(domain) {
		return fmt.Errorf("%q must be lowercase", domain)
	}

	for _, label := range strings.Split(domain, ".") {
		if len(label) > 63 || !dnsLabelPattern.MatchString(label) {
			return fmt.Errorf("%q is not a valid domain name", domain)
		}
	}

	return nil
}
