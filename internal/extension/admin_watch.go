package extension

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/npm"
)

// PrepareAdminWatcher runs the admin watcher preparation steps and returns the
// dev server process. When out is non-nil, the output of every preparation step
// (feature:dump, npm install, schema generation) is streamed to it so the steps
// are not silent while they run.
func PrepareAdminWatcher(ctx context.Context, projectRoot string, cmdExecutor executor.Executor, out io.Writer) (*executor.Process, error) {
	logStep(out, "Dumping features...")
	if err := runStep(ctx, cmdExecutor, out, "feature:dump"); err != nil {
		return nil, err
	}

	adminRelPath := PlatformRelPath(projectRoot, "Administration", "Resources/app/administration")
	adminExecutor := cmdExecutor.WithRelDir(adminRelPath)

	if _, err := os.Stat(PlatformPath(projectRoot, "Administration", "Resources/app/administration/node_modules/webpack-dev-server")); os.IsNotExist(err) {
		logStep(out, "Installing npm dependencies (this can take a few minutes)...")
		if err := npm.InstallDependenciesStreamed(ctx, adminExecutor, npm.NonEmptyPackage, out); err != nil {
			return nil, err
		}
	}

	converterPath := PlatformPath(projectRoot, "Administration", "Resources/app/administration/scripts/entitySchemaConverter/entity-schema-converter.ts")
	if _, err := os.Stat(converterPath); err == nil {
		logStep(out, "Generating entity schema...")
		if err := prepareEntitySchema(ctx, projectRoot, cmdExecutor, adminExecutor, out); err != nil {
			return nil, err
		}
	}

	adminExecutor = adminExecutor.WithEnv(map[string]string{
		"PROJECT_ROOT": projectRoot,
		"ADMIN_ROOT":   PlatformPath(projectRoot, "Administration", ""),
	})

	return adminExecutor.NPMCommand(ctx, "run", "dev"), nil
}

func prepareEntitySchema(ctx context.Context, projectRoot string, cmdExecutor, adminExecutor executor.Executor, out io.Writer) error {
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

	if err := runStep(ctx, cmdExecutor, out, "framework:schema", "-s", "entity-schema", filepath.Join(relMockDir, "entity-schema.json")); err != nil {
		return err
	}

	return runNPMStep(ctx, adminExecutor, out, "run", "convert-entity-schema")
}

// logStep writes a step header to out (when set) so the user can tell which
// preparation step is currently running.
func logStep(out io.Writer, msg string) {
	if out != nil {
		_, _ = fmt.Fprintf(out, "\n> %s\n", msg)
	}
}

// runStep runs a console command, streaming its output to out when set.
func runStep(ctx context.Context, e executor.Executor, out io.Writer, args ...string) error {
	cmd := e.ConsoleCommand(ctx, args...)
	if out != nil {
		return cmd.RunWithOutput(out)
	}
	return cmd.Run()
}

// runNPMStep runs an npm command, streaming its output to out when set.
func runNPMStep(ctx context.Context, e executor.Executor, out io.Writer, args ...string) error {
	cmd := e.NPMCommand(ctx, args...)
	if out != nil {
		return cmd.RunWithOutput(out)
	}
	return cmd.Run()
}
