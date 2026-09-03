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

	// Match assignments the way ExtractEnvVar reads them (whitespace around the
	// key tolerated, comments skipped), so an upsert replaces the line a read
	// would return instead of appending a duplicate that never takes effect.
	replaced := false
	for i, l := range lines {
		if isAssignmentOf(l, key) {
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

// isAssignmentOf reports whether the dotenv line assigns key.
func isAssignmentOf(line, key string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return false
	}
	k, _, ok := strings.Cut(line, "=")
	return ok && strings.TrimSpace(k) == key
}

// ReadEnvVar returns the value of key in the env file at path, or "" when
// the file or the key does not exist.
func ReadEnvVar(path, key string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return ExtractEnvVar(content, key)
}

// ExtractEnvVar returns the value of key in raw dotenv content, or "" when
// unset. Blank and comment lines are skipped, whitespace around key and value
// is trimmed, and the first assignment wins.
func ExtractEnvVar(content []byte, key string) string {
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(k) == key {
			return strings.TrimSpace(value)
		}
	}

	return ""
}
