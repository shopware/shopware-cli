package extension

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/npm"
)

// PrepareAdminWatcher performs all setup steps needed before running the admin
// watcher (feature dump, node_modules, env vars, entity schema) and returns
// the *exec.Cmd for "npm run dev" ready to be started.
func PrepareAdminWatcher(ctx context.Context, projectRoot string, cmdExecutor executor.Executor) (*exec.Cmd, error) {
	if err := cmdExecutor.ConsoleCommand(ctx, "feature:dump").Run(); err != nil {
		return nil, err
	}

	adminRelPath := PlatformRelPath(projectRoot, "Administration", "Resources/app/administration")
	adminExecutor := cmdExecutor.WithRelDir(adminRelPath)

	if _, err := os.Stat(PlatformPath(projectRoot, "Administration", "Resources/app/administration/node_modules/webpack-dev-server")); os.IsNotExist(err) {
		if err := npm.InstallDependencies(ctx, adminExecutor, npm.NonEmptyPackage); err != nil {
			return nil, err
		}
	}

	converterPath := PlatformPath(projectRoot, "Administration", "Resources/app/administration/scripts/entitySchemaConverter/entity-schema-converter.ts")
	if _, err := os.Stat(converterPath); err == nil {
		if err := prepareEntitySchema(ctx, projectRoot, cmdExecutor, adminExecutor); err != nil {
			return nil, err
		}
	}

	adminExecutor = adminExecutor.WithEnv(map[string]string{
		"PROJECT_ROOT": projectRoot,
		"ADMIN_ROOT":   PlatformPath(projectRoot, "Administration", ""),
	})

	return adminExecutor.NPMCommand(ctx, "run", "dev"), nil
}

func prepareEntitySchema(ctx context.Context, projectRoot string, cmdExecutor, adminExecutor executor.Executor) error {
	mockDirectory := PlatformPath(projectRoot, "Administration", "Resources/app/administration/test/_mocks_")
	if _, err := os.Stat(mockDirectory); os.IsNotExist(err) {
		if err := os.MkdirAll(mockDirectory, os.ModePerm); err != nil {
			return err
		}
	}

	relMockDir, err := filepath.Rel(projectRoot, mockDirectory)
	if err != nil {
		return err
	}

	if err := cmdExecutor.ConsoleCommand(ctx, "framework:schema", "-s", "entity-schema", filepath.Join(relMockDir, "entity-schema.json")).Run(); err != nil {
		return err
	}

	return adminExecutor.NPMCommand(ctx, "run", "convert-entity-schema").Run()
}
