package shop

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ComposeProjectNameEnvKey is the host-side Docker Compose project name written
// to the project .env (not .env.local). Compose loads .env automatically when
// commands run with Dir = project root.
const ComposeProjectNameEnvKey = "COMPOSE_PROJECT_NAME"

var nonComposeNameChars = regexp.MustCompile(`[^a-z0-9_-]+`)

// GenerateComposeProjectName builds a unique Compose project name that satisfies
// ProjectNameRule / ValidateProjectName. Format: sw-<basename>-<6 hex>.
// The random suffix avoids volume reuse when a directory is deleted and
// recreated with the same basename.
func GenerateComposeProjectName(projectFolder string) (string, error) {
	base := strings.ToLower(filepath.Base(projectFolder))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "shop"
	}

	base = nonComposeNameChars.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-_")
	if base == "" {
		base = "shop"
	}
	// Compose names must start with a letter or digit.
	if base[0] == '-' || base[0] == '_' {
		base = "x" + base
	}

	var suffix [3]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate compose project name: %w", err)
	}

	name := fmt.Sprintf("sw-%s-%s", base, hex.EncodeToString(suffix[:]))
	if err := ValidateProjectName(name); err != nil {
		return "", err
	}

	return name, nil
}

// EnvFileContent returns the initial host .env contents for a scaffolded project.
// Docker projects get a unique COMPOSE_PROJECT_NAME so Compose volumes are not
// reused across same-basename recreations. Non-docker projects get an empty .env.
func EnvFileContent(useDocker bool, projectFolder string) (string, error) {
	if !useDocker {
		return "", nil
	}

	name, err := GenerateComposeProjectName(projectFolder)
	if err != nil {
		return "", err
	}

	return ComposeProjectNameEnvKey + "=" + name + "\n", nil
}

// ExtractComposeProjectName returns the COMPOSE_PROJECT_NAME value from raw
// dotenv content, or "" when unset.
func ExtractComposeProjectName(envContent []byte) string {
	for _, line := range strings.Split(string(envContent), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == ComposeProjectNameEnvKey {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

// EnsureComposeProjectName writes COMPOSE_PROJECT_NAME into the project .env
// when it is not already set. Existing values are left untouched (no silent
// volume disconnect for established stacks). The file is created when missing.
func EnsureComposeProjectName(projectRoot string) error {
	envPath := filepath.Join(projectRoot, ".env")
	existing, err := os.ReadFile(envPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if ExtractComposeProjectName(existing) != "" {
		return nil
	}

	name, err := GenerateComposeProjectName(projectRoot)
	if err != nil {
		return err
	}

	content := existing
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}
	content = append(content, []byte(ComposeProjectNameEnvKey+"="+name+"\n")...)

	return os.WriteFile(envPath, content, 0o644)
}
