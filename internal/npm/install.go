package npm

import (
	"context"
	"fmt"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/logging"
)

// InstallDependencies runs npm install using the given executor.
// Additional parameters can be passed to customize the install behavior.
func InstallDependencies(ctx context.Context, exec executor.Executor, pkg *Package, additionalParams ...string) error {
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
		"PUPPETEER_SKIP_DOWNLOAD":      "1",
		"NPM_CONFIG_ENGINE_STRICT":     "false",
		"NPM_CONFIG_FUND":              "false",
		"NPM_CONFIG_AUDIT":             "false",
		"NPM_CONFIG_UPDATE_NOTIFIER":   "false",
	})

	installCmd := withEnv.NPMCommand(ctx, args...)

	combinedOutput, err := installCmd.CombinedOutput()
	if err != nil {
		logging.FromContext(context.Background()).Errorf("npm install failed: %s", string(combinedOutput))
		return fmt.Errorf("installing dependencies failed with error: %w", err)
	}

	return nil
}
