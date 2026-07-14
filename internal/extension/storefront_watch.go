package extension

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/npm"
)

type StorefrontWatcherOptions struct {
	ThemeID   string
	DomainURL string
}

// PrepareStorefrontWatcher runs the storefront watcher preparation steps and
// returns the hot-proxy process. When out is non-nil, the output of every
// preparation step (feature:dump, theme:compile, theme:dump, npm install) is
// streamed to it so the steps are not silent while they run.
func PrepareStorefrontWatcher(ctx context.Context, projectRoot string, cmdExecutor executor.Executor, opts StorefrontWatcherOptions, out io.Writer) (*executor.Process, error) {
	logStep(out, "Dumping features...")
	if err := runStep(ctx, cmdExecutor, out, "feature:dump"); err != nil {
		return nil, err
	}

	activeOnly := "--active-only"
	if !themeCompileSupportsActiveOnly(projectRoot) {
		activeOnly = "-v"
	}

	logStep(out, "Compiling theme...")
	if err := runStep(ctx, cmdExecutor, out, "theme:compile", activeOnly); err != nil {
		return nil, err
	}

	dumpArgs := storefrontThemeDumpArgs(opts)

	logStep(out, "Dumping theme...")
	if err := runStep(ctx, cmdExecutor, out, dumpArgs...); err != nil {
		return nil, err
	}

	storefrontRelPath := PlatformRelPath(projectRoot, "Storefront", "Resources/app/storefront")
	storefrontExecutor := cmdExecutor.WithRelDir(storefrontRelPath)

	if _, err := os.Stat(PlatformPath(projectRoot, "Storefront", "Resources/app/storefront/node_modules/webpack-dev-server")); os.IsNotExist(err) {
		logStep(out, "Installing npm dependencies (this can take a few minutes)...")
		if err := npm.InstallDependenciesStreamed(ctx, storefrontExecutor, npm.NonEmptyPackage, out); err != nil {
			return nil, err
		}
	}

	storefrontExecutor = storefrontExecutor.WithEnv(map[string]string{
		"PROJECT_ROOT":    projectRoot,
		"STOREFRONT_ROOT": PlatformPath(projectRoot, "Storefront", ""),
	})

	return storefrontExecutor.NPMCommand(ctx, "run-script", "hot-proxy"), nil
}

func storefrontThemeDumpArgs(opts StorefrontWatcherOptions) []string {
	args := []string{"theme:dump"}
	if opts.ThemeID != "" {
		args = append(args, opts.ThemeID)
		if opts.DomainURL != "" {
			args = append(args, opts.DomainURL)
		}
	} else {
		// Preparation commands do not have stdin attached. Keep the legacy
		// theme:dump behavior without prompting when multiple themes exist.
		args = append(args, "--no-interaction")
	}

	return args
}

func themeCompileSupportsActiveOnly(projectRoot string) bool {
	themeFile := PlatformPath(projectRoot, "Storefront", "Theme/Command/ThemeCompileCommand.php")

	bytes, err := os.ReadFile(themeFile)
	if err != nil {
		return false
	}

	return strings.Contains(string(bytes), "active-only")
}
