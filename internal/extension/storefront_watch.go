package extension

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/npm"
	"github.com/shopware/shopware-cli/internal/system"
)

type StorefrontWatcherOptions struct {
	ThemeID   string
	DomainURL string
}

// PrepareStorefrontWatcher runs the storefront watcher preparation steps and
// returns the hot-proxy process. When out is non-nil, the output of every
// preparation step (feature:dump, theme:compile, theme:dump, npm install) is
// streamed to it so the steps are not silent while they run.
func PrepareStorefrontWatcher(ctx context.Context, projectRoot string, cmdExecutor executor.Executor, opts StorefrontWatcherOptions, in io.Reader, out io.Writer) (*executor.Process, error) {
	dumpArgs, err := storefrontThemeDumpArgs(ctx, opts)
	if err != nil {
		return nil, err
	}

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

	logStep(out, "Dumping theme...")
	if err := runStorefrontThemeDump(ctx, cmdExecutor, in, out, dumpArgs...); err != nil {
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

func storefrontThemeDumpArgs(ctx context.Context, opts StorefrontWatcherOptions) ([]string, error) {
	args := []string{"theme:dump"}
	if opts.ThemeID != "" {
		args = append(args, opts.ThemeID)
		if opts.DomainURL != "" {
			args = append(args, opts.DomainURL)
		}
	} else if !system.IsInteractionEnabled(ctx) {
		return nil, fmt.Errorf("theme selection requires interaction; pass --sales-channel <id> when using --no-interaction")
	}

	return args, nil
}

func runStorefrontThemeDump(ctx context.Context, e executor.Executor, in io.Reader, out io.Writer, args ...string) error {
	cmd := e.ConsoleCommand(ctx, args...)
	cmd.Cmd.Stdin = in
	if out != nil {
		return cmd.RunWithOutput(out)
	}

	return cmd.Run()
}

func themeCompileSupportsActiveOnly(projectRoot string) bool {
	themeFile := PlatformPath(projectRoot, "Storefront", "Theme/Command/ThemeCompileCommand.php")

	bytes, err := os.ReadFile(themeFile)
	if err != nil {
		return false
	}

	return strings.Contains(string(bytes), "active-only")
}
