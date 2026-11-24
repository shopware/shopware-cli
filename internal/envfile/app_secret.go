package envfile

import (
	"fmt"
	"path/filepath"
)

const appSecretEnvKey = "APP_SECRET"

// MoveAppSecretToEnvLocal moves the APP_SECRET that Symfony Flex wrote into
// the committed .env of a new project to the gitignored .env.local, keeping
// its value, and leaves APP_SECRET empty in .env.
func MoveAppSecretToEnvLocal(projectRoot string) error {
	envPath := filepath.Join(projectRoot, ".env")
	secret := ReadEnvVar(envPath, appSecretEnvKey)
	if secret == "" {
		return nil
	}

	if err := UpsertEnvVar(filepath.Join(projectRoot, ".env.local"), appSecretEnvKey, secret); err != nil {
		return fmt.Errorf("cannot write APP_SECRET to .env.local: %w", err)
	}
	if err := UpsertEnvVar(envPath, appSecretEnvKey, ""); err != nil {
		return fmt.Errorf("cannot clear APP_SECRET in .env: %w", err)
	}

	return nil
}
