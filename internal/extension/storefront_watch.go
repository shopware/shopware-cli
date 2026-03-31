package extension

import (
	"context"
	"os"
	"strings"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/npm"
)

// PrepareStorefrontWatcher performs all setup steps needed before running the
// storefront watcher (feature dump, theme compile, theme dump, node_modules,
// env vars) and returns the Process for "npm run-script hot-proxy" ready
// to be started.
func PrepareStorefrontWatcher(ctx context.Context, projectRoot string, cmdExecutor executor.Executor) (*executor.Process, error) {
	if err := cmdExecutor.ConsoleCommand(ctx, "feature:dump").Run(); err != nil {
		return nil, err
	}

	activeOnly := "--active-only"
	if !themeCompileSupportsActiveOnly(projectRoot) {
		activeOnly = "-v"
	}

	if err := cmdExecutor.ConsoleCommand(ctx, "theme:compile", activeOnly).Run(); err != nil {
		return nil, err
	}

	if err := cmdExecutor.ConsoleCommand(ctx, "theme:dump").Run(); err != nil {
		return nil, err
	}

	storefrontRelPath := PlatformRelPath(projectRoot, "Storefront", "Resources/app/storefront")
	storefrontExecutor := cmdExecutor.WithRelDir(storefrontRelPath)

	if _, err := os.Stat(PlatformPath(projectRoot, "Storefront", "Resources/app/storefront/node_modules/webpack-dev-server")); os.IsNotExist(err) {
		if err := npm.InstallDependencies(ctx, storefrontExecutor, npm.NonEmptyPackage); err != nil {
			return nil, err
		}
	}

	storefrontExecutor = storefrontExecutor.WithEnv(map[string]string{
		"PROJECT_ROOT":    projectRoot,
		"STOREFRONT_ROOT": PlatformPath(projectRoot, "Storefront", ""),
	})

	return storefrontExecutor.NPMCommand(ctx, "run-script", "hot-proxy"), nil
}

func themeCompileSupportsActiveOnly(projectRoot string) bool {
	themeFile := PlatformPath(projectRoot, "Storefront", "Theme/Command/ThemeCompileCommand.php")

	bytes, err := os.ReadFile(themeFile)
	if err != nil {
		return false
	}

	return strings.Contains(string(bytes), "active-only")
}
