package npm

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/shopware/shopware-cli/logging"
)

// InstallDependencies runs npm install in the given directory.
// Additional parameters can be passed to customize the install behavior.
func InstallDependencies(ctx context.Context, dir string, pkg *Package, additionalParams ...string) error {
	isProductionMode := false

	for _, param := range additionalParams {
		if param == "--production" {
			isProductionMode = true
		}
	}

	if isProductionMode && pkg != nil && len(pkg.Dependencies) == 0 {
		return nil
	}

	installCmd := exec.CommandContext(ctx, "npm", "install", "--no-audit", "--no-fund", "--prefer-offline", "--loglevel=error")
	installCmd.Args = append(installCmd.Args, additionalParams...)
	installCmd.Dir = dir
	installCmd.Env = os.Environ()
	installCmd.Env = append(installCmd.Env,
		"PUPPETEER_SKIP_DOWNLOAD=1",
		"NPM_CONFIG_ENGINE_STRICT=false",
		"NPM_CONFIG_FUND=false",
		"NPM_CONFIG_AUDIT=false",
		"NPM_CONFIG_UPDATE_NOTIFIER=false",
	)

	combinedOutput, err := installCmd.CombinedOutput()
	if err != nil {
		logging.FromContext(context.Background()).Errorf("npm install failed in %s: %s", dir, string(combinedOutput))
		return fmt.Errorf("installing dependencies for %s failed with error: %w", dir, err)
	}

	return nil
}

// RunScript runs an npm script in the given directory with optional environment variables.
func RunScript(ctx context.Context, dir string, script string, env []string) error {
	npmCmd := exec.CommandContext(ctx, "npm", "--prefix", dir, "run", script) //nolint:gosec
	npmCmd.Env = os.Environ()
	npmCmd.Env = append(npmCmd.Env, env...)
	npmCmd.Stdout = os.Stdout
	npmCmd.Stderr = os.Stderr

	return npmCmd.Run()
}
