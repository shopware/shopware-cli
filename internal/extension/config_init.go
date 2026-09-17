package extension

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/compatibility"
	"github.com/shopware/shopware-cli/logging"
)

const (
	// ConfigSchemaURL is the JSON Schema URL used by the YAML language server.
	ConfigSchemaURL = "https://shopware.github.io/shopware-cli/shopware-extension-schema.json"
)

var ConfigLocations = []string{
	".config/shopware-extension.yml", // recommended location
	".shopware-extension.yml",
	".shopware-extension.yaml",
}

// EmptyConfigFile returns the default content written by `extension config init`:
// yaml-language-server schema comment plus today's compatibility_date.
// All other keys remain optional.
func EmptyConfigFile() string {
	return fmt.Sprintf(
		"# yaml-language-server: $schema=%s\ncompatibility_date: %s\n",
		ConfigSchemaURL,
		compatibility.TodayDate(),
	)
}

// ConfigPath returns the path of an existing config file, or "" if none.
// It logs warnings if further config files exists that aren't used
func ConfigPath(ctx context.Context, dir string) string {
	for idx, name := range ConfigLocations {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err != nil {
			continue
		}

		if idx >= len(ConfigLocations)-1 {
			// no further locations to check, so no warnings needed
			return p
		}

		// found config, but before returning check others and warn if they exists
		logger := logging.FromContext(ctx)
		for _, furherLoc := range ConfigLocations[idx+1:] {
			furtherConfigPath := filepath.Join(dir, furherLoc)
			if _, err := os.Stat(furtherConfigPath); err == nil {
				logger.Warnf(
					"Unused config found %s, the loaded config is %s",
					furtherConfigPath,
					p,
				)
			}
		}

		return p
	}

	return ""
}

// ConfigExists reports whether a .config/shopware-extension.yml or .shopware-extension.yml/.yaml is present.
func ConfigExists(ctx context.Context, dir string) bool {
	return ConfigPath(ctx, dir) != ""
}

// InitConfig writes a minimal .config/shopware-extension.yml with the YAML language
// server schema comment and today's compatibility_date. All other keys are optional.
//
// If a config already exists and force is false, an error is returned.
// Returns the path of the written file.
func InitConfig(ctx context.Context, dir string, force bool) (string, error) {
	path := ConfigPath(ctx, dir)
	if path != "" && !force {
		return "", fmt.Errorf("%s already exists (pass --force to overwrite)", path)
	}

	if path == "" {
		path = filepath.Join(dir, ConfigLocations[0])
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(EmptyConfigFile()), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}

	return path, nil
}
