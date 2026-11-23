package envfile

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
)

// appSecretEnvKey is the Symfony secret. It belongs in the gitignored
// .env.local so it is never committed with .env.
const appSecretEnvKey = "APP_SECRET"

// EnsureAppSecret writes a random APP_SECRET to the project's .env.local
// unless that file already defines a non-empty one. Values in other env files
// are ignored. It reports whether a secret was generated.
func EnsureAppSecret(projectRoot string) (bool, error) {
	path := filepath.Join(projectRoot, ".env.local")
	if ReadEnvVar(path, appSecretEnvKey) != "" {
		return false, nil
	}

	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return false, fmt.Errorf("cannot generate APP_SECRET: %w", err)
	}

	if err := UpsertEnvVar(path, appSecretEnvKey, hex.EncodeToString(value)); err != nil {
		return false, fmt.Errorf("cannot write APP_SECRET to .env.local: %w", err)
	}

	return true, nil
}

// KeepAppSecretOutOfEnv makes sure the committed .env holds no secret: it
// generates a fresh APP_SECRET in the gitignored .env.local and blanks the
// one Symfony Flex wrote into .env. The .env value is discarded, not moved.
func KeepAppSecretOutOfEnv(projectRoot string) error {
	if _, err := EnsureAppSecret(projectRoot); err != nil {
		return err
	}

	envPath := filepath.Join(projectRoot, ".env")
	if ReadEnvVar(envPath, appSecretEnvKey) == "" {
		return nil
	}
	if err := UpsertEnvVar(envPath, appSecretEnvKey, ""); err != nil {
		return fmt.Errorf("cannot clear APP_SECRET in .env: %w", err)
	}

	return nil
}
