package envfile

import (
	"fmt"
	"os"
	"strings"
)

// UpsertEnvVar sets key to value in the given dotenv file, replacing an
// existing assignment or appending a new one. The file is created when it
// does not exist. All other lines are kept untouched.
func UpsertEnvVar(path, key, value string) error {
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	line := fmt.Sprintf("%s=%s", key, value)
	replaced := false

	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")

	for i, existing := range lines {
		trimmed := strings.TrimSpace(existing)
		trimmed = strings.TrimPrefix(trimmed, "export ")

		if strings.HasPrefix(trimmed, key+"=") {
			lines[i] = line
			replaced = true
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
