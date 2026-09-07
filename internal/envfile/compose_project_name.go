package envfile

import (
	"os"
	"path/filepath"
)

// ComposeProjectNameEnvKey is the host-side Docker Compose project name written
// to the project .env (not .env.local). Compose loads .env automatically when
// commands run with Dir = project root.
const ComposeProjectNameEnvKey = "COMPOSE_PROJECT_NAME"

// ReadComposeProjectName returns the COMPOSE_PROJECT_NAME configured in the
// project's .env, or "" when the file or the key is missing.
func ReadComposeProjectName(projectRoot string) string {
	content, err := os.ReadFile(filepath.Join(projectRoot, ".env"))
	if err != nil {
		return ""
	}
	return ExtractComposeProjectName(content)
}

// ExtractComposeProjectName returns the COMPOSE_PROJECT_NAME value from raw
// dotenv content, or "" when unset.
func ExtractComposeProjectName(envContent []byte) string {
	return ExtractEnvVar(envContent, ComposeProjectNameEnvKey)
}
