// Package userconfig loads the optional user-level shopware-cli config file.
package userconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PathEnv overrides the config file location, mainly for tests.
const PathEnv = "SHOPWARE_CLI_CONFIG"

// Config holds user preferences. The zero value matches the defaults:
// telemetry and update hints enabled.
type Config struct {
	Telemetry    *bool `yaml:"telemetry"`
	NoUpdateHint bool  `yaml:"no_update_hint"`
}

// TelemetryEnabled reports whether the config allows telemetry.
func (c Config) TelemetryEnabled() bool {
	return c.Telemetry == nil || *c.Telemetry
}

// Path returns the config file location: SHOPWARE_CLI_CONFIG if set,
// otherwise <user config dir>/shopware-cli/config.yaml.
func Path() (string, error) {
	if path := os.Getenv(PathEnv); path != "" {
		return path, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "shopware-cli", "config.yaml"), nil
}

// Load reads the user config. A missing file yields the defaults. The file is
// never created or written.
func Load() (Config, error) {
	var cfg Config

	path, err := Path()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}

	return cfg, nil
}
