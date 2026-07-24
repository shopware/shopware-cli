package flexmigrator

import (
	"os"
	"path"

	"github.com/shopware/shopware-cli/internal/shop"
)

func MigrateEnv(project string) error {
	envPath := path.Join(project, ".env")
	envLocalPath := path.Join(project, ".env.local")

	_, envLocalErr := os.Stat(envLocalPath)
	_, envErr := os.Stat(envPath)

	if os.IsNotExist(envLocalErr) && !os.IsNotExist(envErr) {
		// Preserve host-side Compose project name across the flex split:
		// application secrets move to .env.local; COMPOSE_PROJECT_NAME stays in .env.
		envBytes, err := os.ReadFile(envPath)
		if err != nil {
			return err
		}
		composeName := shop.ExtractComposeProjectName(envBytes)

		if err := os.Rename(envPath, envLocalPath); err != nil {
			return err
		}

		newEnv := ""
		if composeName != "" {
			newEnv = shop.ComposeProjectNameEnvKey + "=" + composeName + "\n"
		}

		return os.WriteFile(envPath, []byte(newEnv), 0o644)
	}

	return nil
}
