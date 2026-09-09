package projectbuild

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/internal/ci"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

// defaultCleanupPaths are paths not needed for the production build.
var defaultCleanupPaths = []string{
	"vendor/shopware/storefront/Resources/app/storefront/vendor/bootstrap/dist",
	"vendor/shopware/storefront/Resources/app/storefront/test",
	"vendor/shopware/storefront/Test",
	"vendor/shopware/core/Framework/Test",
	"vendor/shopware/core/Content/Test",
	"vendor/shopware/core/Checkout/Test",
	"vendor/shopware/core/System/Test",
	"vendor/tecnickcom/tcpdf/examples",
}

func buildCleanupPaths(cfg *shop.ConfigBuild) []string {
	return slices.Concat(defaultCleanupPaths, cfg.CleanupPaths)
}

// Build defaults belong to child processes, not the CLI's process environment.
func buildEnvironment(getenv func(string) string) map[string]string {
	env := map[string]string{
		"APP_ENV":               getenv("APP_ENV"),
		"COMPOSER_ROOT_VERSION": getenv("COMPOSER_ROOT_VERSION"),
		"SHOPWARE_SKIP_ASSET_INSTALL_CACHE_INVALIDATION": "1",
	}
	if env["APP_ENV"] == "" {
		env["APP_ENV"] = "prod"
	}
	if env["COMPOSER_ROOT_VERSION"] == "" {
		env["COMPOSER_ROOT_VERSION"] = "1.0.0"
	}
	return env
}

func createEmptySnippetFolder(root string) error {
	dirs := []string{
		"Resources/app/administration/src/app/snippet",
		"Resources/app/administration/src/module/dummy/snippet",
		"Resources/app/administration/src/app/component/dummy/dummy/snippet",
	}
	for _, dir := range dirs {
		fullPath := path.Join(root, dir)
		if err := os.MkdirAll(fullPath, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path.Join(fullPath, ".gitkeep"), []byte{}, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// CI historically skips SBOM generation when composer.lock is missing.
func generateProjectSBOM(ctx context.Context, root, toolVersion string) error {
	section := ci.Default.Section(ctx, "Generating SBOM")
	defer section.End(ctx)
	return shop.WriteProjectSBOM(ctx, root, shop.ProjectSBOMOptions{
		SkipMissingLock: true,
		ToolVersion:     toolVersion,
	})
}

func prepareComposerAuth(ctx context.Context, root string) (string, error) {
	auth, err := shop.ReadComposerAuth(path.Join(root, "auth.json"))
	if err != nil {
		logging.FromContext(ctx).Warnf("Failed to read composer auth from env: %v", err)
		return "", err
	}
	data, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RunCommand runs a build or asset watcher process with inherited terminal I/O
// and the fixed, non-production secret and lock defaults needed without a shop.
// Environment variables already supplied by the executor are preserved.
func RunCommand(p *executor.Process) error {
	p.Cmd.Stdin = os.Stdin
	p.Cmd.Stdout = os.Stdout
	p.Cmd.Stderr = os.Stderr
	applyTransparentEnv(p)
	return p.Run()
}

func applyTransparentEnv(p *executor.Process) {
	if p.Cmd.Env == nil {
		p.Cmd.Env = os.Environ()
	}
	p.Cmd.Env = append(p.Cmd.Env, "APP_SECRET=b59a3a283700fde2162c0d4f2bcf2588c3e841ef1976cf042d8500c3f3152ec513f77453797387dc004ff399cce0d3663e4fec770e6f11aa4ccd2846854c3a9f", "LOCK_DSN=flock")
}

func binCICommand(ctx context.Context, cmdExecutor executor.Executor, args ...string) *executor.Process {
	return cmdExecutor.PHPCommand(ctx, append([]string{"bin/ci"}, args...)...)
}

func cleanupTcpdf(folder string, ctx context.Context) error {
	tcpdfPath := path.Join(folder, "vendor", "tecnickcom/tcpdf/fonts")
	if _, err := os.Stat(tcpdfPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	logging.FromContext(ctx).Infof("Remove unnecessary fonts from tcpdf")
	return filepath.WalkDir(tcpdfPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == ".z" {
			return os.Remove(path)
		}
		baseName := filepath.Base(path)
		if strings.Contains(baseName, "courier") || strings.Contains(baseName, "helvetica") {
			return nil
		}
		return os.Remove(path)
	})
}

func convertForceExtensionBuild(configExtensions []shop.ConfigBuildExtension) []string {
	extensionConfigs := make([]string, len(configExtensions))
	for i, ext := range configExtensions {
		extensionConfigs[i] = ext.Name
	}
	return extensionConfigs
}

func executeCIHooks(ctx context.Context, sectionName string, hooks []string, root string, env map[string]string) error {
	section := ci.Default.Section(ctx, sectionName)
	defer section.End(ctx)
	for _, hook := range hooks {
		logging.FromContext(ctx).Infof("Running hook: %s", hook)
		hookCmd := exec.CommandContext(ctx, "sh", "-c", hook)
		hookCmd.Stdout = os.Stdout
		hookCmd.Stderr = os.Stderr
		hookCmd.Dir = root
		hookCmd.Env = append(os.Environ(), "PROJECT_ROOT="+root)
		for key, value := range env {
			hookCmd.Env = append(hookCmd.Env, key+"="+value)
		}
		if err := hookCmd.Run(); err != nil {
			return fmt.Errorf("hook failed (%s): %w", hook, err)
		}
	}
	return nil
}
