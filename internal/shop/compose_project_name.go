package shop

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/shopware/shopware-cli/internal/envfile"
)

var nonComposeNameChars = regexp.MustCompile(`[^a-z0-9_-]+`)

// GenerateComposeProjectName builds a unique Compose project name.
// Format: sw-<basename>-<6 hex>. The random suffix avoids volume reuse when a
// directory is deleted and recreated with the same basename. The generated name
// is a valid Compose project name by construction.
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

	var suffix [3]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate compose project name: %w", err)
	}

	name := fmt.Sprintf("sw-%s-%s", base, hex.EncodeToString(suffix[:]))

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

	return envfile.ComposeProjectNameEnvKey + "=" + name + "\n", nil
}

// EnsureComposeProjectName writes COMPOSE_PROJECT_NAME into the project .env
// when it is not already set. Existing values are left untouched (no silent
// volume disconnect for established stacks). The file is created when missing.
func EnsureComposeProjectName(projectRoot string) error {
	name, err := GenerateComposeProjectName(projectRoot)
	if err != nil {
		return err
	}
	return RestoreComposeProjectName(projectRoot, name)
}

// RestoreComposeProjectName re-adds a known COMPOSE_PROJECT_NAME to the
// project .env when the key was lost — e.g. after `composer recipes:install
// --force --reset` replaced .env with the recipe template. A present value is
// left untouched, an empty name is a no-op, and the file is created when
// missing.
func RestoreComposeProjectName(projectRoot, name string) error {
	if name == "" {
		return nil
	}

	envPath := filepath.Join(projectRoot, ".env")
	existing, err := os.ReadFile(envPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if envfile.ExtractComposeProjectName(existing) != "" {
		return nil
	}

	content := existing
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}
	content = append(content, []byte(envfile.ComposeProjectNameEnvKey+"="+name+"\n")...)

	return os.WriteFile(envPath, content, 0o644)
}
