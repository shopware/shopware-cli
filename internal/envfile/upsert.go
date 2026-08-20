package envfile

import (
	"fmt"
	"os"
	"strings"
)

// UpsertEnvVar sets key to value in the env file at path, replacing an
// existing assignment or appending one. The file is created when missing;
// all other lines are preserved as-is.
func UpsertEnvVar(path, key, value string) error {
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	line := fmt.Sprintf("%s=%s", key, value)
	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")

	replaced := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), key+"=") {
			lines[i] = line
			replaced = true
			break
		}
	}

	if !replaced {
		if len(lines) == 1 && lines[0] == "" {
			lines[0] = line
		} else {
			lines = append(lines, line)
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// ReadEnvVar returns the value of key in the env file at path, or "" when
// the file or the key does not exist.
func ReadEnvVar(path, key string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	for _, l := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), key+"=") {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), key+"="))
		}
	}

	return ""
}
