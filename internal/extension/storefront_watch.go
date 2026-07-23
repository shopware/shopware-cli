package extension

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/npm"
)

// storefrontHMRPatch is a Node preload (node --require) injected only when the
// storefront watcher runs behind the shared proxy. It rewrites the deprecated
// webpack hot-reload websocket target to the proxy hostname without touching
// the vendor code. See the file itself for the why.
//
//go:embed storefront_hmr_patch.cjs
var storefrontHMRPatch []byte

const (
	// storefrontHMRPatchFile is where the preload is written inside the project
	// (under var/, which is bind-mounted into the container and disposable).
	storefrontHMRPatchFile = "var/shopware-cli-storefront-hmr.cjs"

	// storefrontProxyPort / storefrontAssetsPort are the container ports the
	// hot-proxy's two servers listen on: the HTML proxy and the asset+HMR
	// server. In proxy mode Traefik routes storefront-watch.<host> to the
	// former (websecure) and :<assets> to the latter (the sfassets entrypoint),
	// so these must match the routes in internal/docker/compose_override.go and
	// the Traefik entrypoint in internal/proxy/traefik.go.
	storefrontProxyPort  = 9998
	storefrontAssetsPort = 8443
)

type StorefrontWatcherOptions struct {
	ThemeID   string
	DomainURL string
	// ProxyHostname, when set, routes the deprecated webpack hot-proxy watcher
	// through the shared proxy at this hostname (e.g.
	// "storefront-watch.my-shop.shopware.local") instead of exposing fixed
	// local ports, so multiple shops can watch in parallel. Empty keeps the
	// classic local-port behavior.
	ProxyHostname string
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

	dumpArgs := []string{"theme:dump"}
	if opts.ThemeID != "" {
		dumpArgs = append(dumpArgs, opts.ThemeID)
		if opts.DomainURL != "" {
			dumpArgs = append(dumpArgs, opts.DomainURL)
		}
	}

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

	env := map[string]string{
		"PROJECT_ROOT":    projectRoot,
		"STOREFRONT_ROOT": PlatformPath(projectRoot, "Storefront", ""),
	}

	if opts.ProxyHostname != "" {
		proxyEnv, err := storefrontProxyEnv(projectRoot, cmdExecutor, opts.ProxyHostname)
		if err != nil {
			return nil, err
		}
		for k, v := range proxyEnv {
			env[k] = v
		}
	}

	return storefrontExecutor.WithEnv(env).NPMCommand(ctx, "run-script", "hot-proxy"), nil
}

// storefrontProxyEnv writes the hot-reload websocket preload and returns the
// environment that runs the deprecated webpack hot-proxy through the shared
// proxy: the two servers bind their proxy ports, TLS is left to Traefik
// (STOREFRONT_SKIP_SSL_CERT), and the preload redirects the browser's HMR
// websocket to the proxy hostname.
func storefrontProxyEnv(projectRoot string, cmdExecutor executor.Executor, proxyHostname string) (map[string]string, error) {
	hostPatchPath := filepath.Join(projectRoot, storefrontHMRPatchFile)
	if err := os.MkdirAll(filepath.Dir(hostPatchPath), 0o755); err != nil {
		return nil, fmt.Errorf("preparing storefront watcher patch dir: %w", err)
	}
	if err := os.WriteFile(hostPatchPath, storefrontHMRPatch, 0o644); err != nil {
		return nil, fmt.Errorf("writing storefront watcher patch: %w", err)
	}

	return map[string]string{
		"PROXY_URL":                "https://" + proxyHostname,
		"STOREFRONT_PROXY_PORT":    strconv.Itoa(storefrontProxyPort),
		"STOREFRONT_ASSETS_PORT":   strconv.Itoa(storefrontAssetsPort),
		"STOREFRONT_SKIP_SSL_CERT": "true",
		"NODE_OPTIONS":             "--require " + cmdExecutor.NormalizePath(hostPatchPath),
		"SHOPWARE_CLI_HMR_WS_HOST": proxyHostname,
		"SHOPWARE_CLI_HMR_WS_PORT": strconv.Itoa(storefrontAssetsPort),
	}, nil
}

func themeCompileSupportsActiveOnly(projectRoot string) bool {
	themeFile := PlatformPath(projectRoot, "Storefront", "Theme/Command/ThemeCompileCommand.php")

	bytes, err := os.ReadFile(themeFile)
	if err != nil {
		return false
	}

	return strings.Contains(string(bytes), "active-only")
}
