package extension

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/compatibility"
)

const (
	// ConfigFileName is the canonical extension config filename.
	ConfigFileName = ".shopware-extension.yml"
	// ConfigFileNameAlt is accepted when reading existing configs.
	ConfigFileNameAlt = ".shopware-extension.yaml"

	// ConfigSchemaURL is the JSON Schema URL used by the YAML language server.
	ConfigSchemaURL = "https://shopware.github.io/shopware-cli/shopware-extension-schema.json"
)

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
func ConfigPath(dir string) string {
	for _, name := range []string{ConfigFileName, ConfigFileNameAlt} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// ConfigExists reports whether a .shopware-extension.yml/.yaml is present.
func ConfigExists(dir string) bool {
	return ConfigPath(dir) != ""
}

// InitConfig writes a minimal .shopware-extension.yml with the YAML language
// server schema comment and today's compatibility_date. All other keys are optional.
//
// If a config already exists and force is false, an error is returned.
// Returns the path of the written file.
func InitConfig(dir string, force bool) (string, error) {
	if existing := ConfigPath(dir); existing != "" && !force {
		return "", fmt.Errorf("%s already exists (pass --force to overwrite)", existing)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, []byte(EmptyConfigFile()), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}

	return path, nil
}
