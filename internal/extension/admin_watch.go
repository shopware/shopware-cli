package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/npm"
)

// Default ports of the administration dev server, chosen by the platform's
// build tooling.
const (
	AdminVitePort    = 5173
	AdminWebpackPort = 8080
)

// AdminDevServerPort returns the port of the dev server that
// PrepareAdminWatcher's `npm run dev` starts, mirroring how the platform's
// own dev script picks its tooling: Shopware 6.7+ ships Vite only (5173),
// while 6.6 ships both and decides at runtime via the ADMIN_VITE feature flag
// in var/config_js_features.json, defaulting to webpack-dev-server (8080).
func AdminDevServerPort(projectRoot string) int {
	adminApp := PlatformPath(projectRoot, "Administration", "Resources/app/administration")
	if _, err := os.Stat(filepath.Join(adminApp, "webpack.config.js")); err != nil {
		// Without a webpack config (6.7+, or platform not installed yet)
		// `npm run dev` can only start Vite.
		return AdminVitePort
	}
	if adminViteFeatureEnabled(projectRoot) {
		return AdminVitePort
	}
	return AdminWebpackPort
}

// adminViteFeatureEnabled reads the ADMIN_VITE flag the way 6.6's dev script
// does (`jq -r '.ADMIN_VITE' var/config_js_features.json`): only an explicit
// true switches to Vite — a missing file or key means webpack.
func adminViteFeatureEnabled(projectRoot string) bool {
	content, err := os.ReadFile(filepath.Join(projectRoot, "var", "config_js_features.json"))
	if err != nil {
		return false
	}

	var flags map[string]any
	if err := json.Unmarshal(content, &flags); err != nil {
		return false
	}

	enabled, _ := flags["ADMIN_VITE"].(bool)
	return enabled
}

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
