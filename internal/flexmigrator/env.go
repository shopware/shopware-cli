package flexmigrator

import (
	"os"
	"path"
)

func MigrateEnv(project string) error {
	_, envLocalErr := os.Stat(path.Join(project, ".env.local"))
	_, envErr := os.Stat(path.Join(project, ".env"))

	if os.IsNotExist(envLocalErr) && !os.IsNotExist(envErr) {
		if err := os.Rename(path.Join(project, ".env"), path.Join(project, ".env.local")); err != nil {
			return err
		}

		return os.WriteFile(path.Join(project, ".env"), []byte(""), os.ModePerm)
	}

	return nil
}
