package npm

import (
	"context"
	"fmt"
	"io"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/logging"
)

// InstallDependencies runs npm install using the given executor.
// Additional parameters can be passed to customize the install behavior.
func InstallDependencies(ctx context.Context, exec executor.Executor, pkg *Package, additionalParams ...string) error {
	return InstallDependenciesStreamed(ctx, exec, pkg, nil, additionalParams...)
}

// InstallDependenciesStreamed behaves like InstallDependencies but, when out is
// non-nil, streams the combined npm output to it live instead of buffering it
// and only surfacing it on failure.
func InstallDependenciesStreamed(ctx context.Context, exec executor.Executor, pkg *Package, out io.Writer, additionalParams ...string) error {
	isProductionMode := false

	for _, param := range additionalParams {
		if param == "--production" {
			isProductionMode = true
		}
	}

	if isProductionMode && pkg != nil && len(pkg.Dependencies) == 0 {
		return nil
	}

	args := []string{"install", "--no-audit", "--no-fund", "--prefer-offline", "--loglevel=error"}
	args = append(args, additionalParams...)

	withEnv := exec.WithEnv(map[string]string{
		"PUPPETEER_SKIP_DOWNLOAD":    "1",
		"NPM_CONFIG_ENGINE_STRICT":   "false",
		"NPM_CONFIG_FUND":            "false",
		"NPM_CONFIG_AUDIT":           "false",
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",
	})

	installProcess := withEnv.NPMCommand(ctx, args...)

	if out != nil {
		if err := installProcess.RunWithOutput(out); err != nil {
			return fmt.Errorf("installing dependencies failed with error: %w", err)
		}
		return nil
	}

	combinedOutput, err := installProcess.CombinedOutput()
	if err != nil {
		logging.FromContext(context.Background()).Errorf("npm install failed: %s", string(combinedOutput))
		return fmt.Errorf("installing dependencies failed with error: %w", err)
	}

	return nil
}
