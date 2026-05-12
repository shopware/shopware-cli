package extension

import (
	"context"
	"os"
	"strings"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/npm"
)

type StorefrontWatcherOptions struct {
	ThemeID   string
	DomainURL string
}

func PrepareStorefrontWatcher(ctx context.Context, projectRoot string, cmdExecutor executor.Executor, opts StorefrontWatcherOptions) (*executor.Process, error) {
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

	dumpArgs := []string{"theme:dump"}
	if opts.ThemeID != "" {
		dumpArgs = append(dumpArgs, opts.ThemeID)
		if opts.DomainURL != "" {
			dumpArgs = append(dumpArgs, opts.DomainURL)
		}
	}

	if err := cmdExecutor.ConsoleCommand(ctx, dumpArgs...).Run(); err != nil {
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
