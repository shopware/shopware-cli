package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shyim/go-composer"
	"github.com/spf13/cobra"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
)

// defaultShopURL is the URL projects use outside proxy mode, matching the
// fixed 8000:8000 port mapping of the standard dev environment.
const defaultShopURL = "http://127.0.0.1:8000"

// ErrProxyNotRegistered is returned by `project proxy status` when the
// current project is not registered with the shared proxy.
var ErrProxyNotRegistered = errors.New("project is not registered with the shared proxy")

type proxyEnvironment struct {
	projectRoot   string
	canonicalRoot string
	configPath    string
	cfg           *shop.Config
	// baseDomain is the machine-wide proxy domain from the settings file,
	// e.g. "shopware.local".
	baseDomain string
	hostname   string
	executor   executor.Executor
}

var projectProxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Reach local instances via stable hostnames instead of ports",
	Long: `Manages a shared local reverse proxy (Traefik) that routes stable per-project
hostnames like https://my-shop.shopware.local to your local Shopware instances,
so any number of shops run in parallel without juggling ports.

New projects can opt in at creation with "project create --local-domain"; their
"project dev" then serves them through the proxy automatically, no "proxy up"
needed. Use "proxy up" to opt an existing port-based project in, and "proxy
down" to revert it. Run "proxy setup" once per machine first (DNS + trust).`,
}

// newProxyEnvironment resolves the current project, its hostname and its
// Docker executor. It requires Docker, since the shared proxy is Docker-only.
func newProxyEnvironment(cmd *cobra.Command) (*proxyEnvironment, error) {
	projectRoot, err := findClosestShopwareProject()
	if err != nil {
		return nil, err
	}

	if err := system.ValidateProjectDependencies(cmd.Context(), true, nil, "manage the shared proxy", "", ""); err != nil {
		return nil, err
	}

	return newProxyEnvironmentForRoot(cmd.Context(), projectRoot, projectConfigPath)
}

// newProxyEnvironmentForRoot builds the proxy environment for an explicit
// project root, used by `proxy teardown` to run down for every registered
// project regardless of the current directory.
func newProxyEnvironmentForRoot(ctx context.Context, projectRoot, configPath string) (*proxyEnvironment, error) {
	cfg, err := shop.ReadConfig(ctx, configPath, true)
	if err != nil {
		return nil, err
	}

	settings, err := proxy.LoadSettings()
	if err != nil {
		return nil, err
	}
	baseDomain := settings.BaseDomain()

	hostname, err := proxy.ProjectHostname(projectRoot, cfg, baseDomain)
	if err != nil {
		return nil, err
	}

	envCfg, err := cfg.ResolveEnvironment(environmentName)
	if err != nil {
		return nil, err
	}

	exec, err := executor.New(projectRoot, envCfg, cfg)
	if err != nil {
		return nil, err
	}

	return &proxyEnvironment{
		projectRoot:   projectRoot,
		canonicalRoot: proxy.CanonicalProjectRoot(projectRoot),
		configPath:    configPath,
		cfg:           cfg,
		baseDomain:    baseDomain,
		hostname:      hostname,
		executor:      exec,
	}, nil
}

func (e *proxyEnvironment) up(cmd *cobra.Command) error {
	ctx := cmd.Context()

	reg, err := proxy.LoadRegistry()
	if err != nil {
		return err
	}

	var certInfo proxy.CertInfo
	err = runStep(ctx, "Starting shared proxy...", func(ctx context.Context) error {
		var infraErr error
		certInfo, infraErr = proxy.PrepareInfra(ctx, e.infraParams(), reg)
		return infraErr
	})
	if err != nil {
		return err
	}

	// Regenerate compose.yaml in proxy mode (no host ports, shared network,
	// Traefik labels, combined CA bundle mounted) before starting the containers.
	if err := dockerpkg.WriteComposeFile(e.projectRoot, e.composeOptions()); err != nil {
		return err
	}

	proxyURL := "https://" + e.hostname

	// Capture the pre-proxy APP_URL (so "proxy down" can restore it) and whether
	// the URL actually changes (only then is the costly theme recompile needed).
	// Both read .env.local before it is rewritten just below.
	previousAppURL := defaultShopURL
	if entry, found := reg.Find(e.canonicalRoot); found && entry.PreviousAppURL != "" {
		previousAppURL = entry.PreviousAppURL
	} else if current := envfile.ReadEnvVar(e.envLocalPath(), "APP_URL"); current != "" {
		previousAppURL = current
	}
	// Never record the proxy URL itself as the pre-proxy state (e.g. a
	// born-proxy project whose .env.local already points at the hostname); that
	// would make "proxy down" try to restore to the proxy URL.
	if previousAppURL == proxyURL {
		previousAppURL = defaultShopURL
	}
	urlChanged := envfile.ReadEnvVar(e.envLocalPath(), "APP_URL") != proxyURL

	// Point .env.local at the proxy hostname before the container starts, so it
	// boots on the right APP_URL. .env.local stays the single, editable source
	// of truth — no pinned env var silently overriding it.
	if err := envfile.UpsertEnvVar(e.envLocalPath(), "APP_URL", proxyURL); err != nil {
		return fmt.Errorf("setting APP_URL in .env.local: %w", err)
	}

	start := time.Now()
	err = runStep(ctx, "Starting development environment...", func(ctx context.Context) error {
		return e.executor.StartEnvironment(ctx)
	})
	if err != nil {
		return fmt.Errorf("starting environment: %w", err)
	}
	elapsed := time.Since(start).Round(time.Millisecond)

	// Repoint the sales channel domain to the proxy hostname; needs the running
	// database, so it happens after the environment is up.
	if err := e.repointSalesChannel(ctx, []string{previousAppURL, "http://" + e.hostname}, proxyURL); err != nil {
		fmt.Println(tui.RedText.Render("  Could not update the shop URL: " + err.Error()))
	}

	if urlChanged {
		e.recompileTheme(ctx)
	}

	entry := proxy.ProjectEntry{
		ProjectRoot:    e.canonicalRoot,
		Hostname:       e.hostname,
		RegisteredAt:   time.Now(),
		PreviousAppURL: previousAppURL,
		PreviousConfig: e.switchProjectConfigURLs(reg, proxyURL),
	}

	reg.Upsert(entry)
	if err := reg.Save(); err != nil {
		return err
	}

	// Make the shop reachable at its own hostname from inside its containers,
	// so self-calls to APP_URL (app callbacks, sitemap, ...) resolve back to
	// it over TLS; as a side effect every registered shop can reach the
	// others too.
	if err := proxy.ReconcileHostnames(ctx, reg.Hostnames()); err != nil {
		fmt.Println(tui.RedText.Render("  Could not register in-container hostnames: " + err.Error()))
	}

	fmt.Println(tui.GreenText.Bold(true).Render(fmt.Sprintf("  ✓ Registered with the shared proxy in %s", elapsed)))
	fmt.Println()
	fmt.Println(tui.SectionTitleStyle.Render("  Shop"))
	fmt.Println(tui.DimText.Render("  Shop URL:  ") + tui.BoldText.Render(proxyURL))
	fmt.Println(tui.DimText.Render("  Admin URL: ") + tui.BoldText.Render(proxyURL+"/admin"))
	fmt.Println()
	fmt.Println(tui.DimText.Render("  Run ") + tui.BoldText.Render("shopware-cli project proxy down") + tui.DimText.Render(" to stop it."))
	fmt.Println()

	e.ensureHostnameResolves(ctx)

	if certInfo.CACreated {
		fmt.Println(tui.DimText.Render("  A local certificate authority was created. Run ") + tui.BoldText.Render("shopware-cli project proxy setup") + tui.DimText.Render(" once so browsers trust it (needs sudo)."))
		fmt.Println()
	}

	maybePrintWSLWindowsAccess(proxyBrowserHostnames(e.projectRoot, e.hostname))

	return nil
}

// proxyBrowserHostnames returns the shop's proxy hostnames (root + routed
// subdomains) for the WSL Windows hosts line, reading the optional AMQP and
// Elasticsearch services from the project's composer.lock.
func proxyBrowserHostnames(projectRoot, hostname string) []string {
	hasAMQP, hasElasticsearch := false, false
	if lock, err := composer.ReadLock(filepath.Join(projectRoot, "composer.lock")); err == nil {
		hasAMQP = lock.GetPackage("symfony/amqp-messenger") != nil
		hasElasticsearch = lock.GetPackage("shopware/elasticsearch") != nil
	}
	return proxy.ProxyHostnames(hostname, hasAMQP, hasElasticsearch)
}

// maybePrintWSLWindowsAccess prints the one-time Windows-side steps (import the
// CA, add the hosts entries) needed to reach proxy shops from a browser on
// Windows, but only when running under WSL — the machine setup configures just
// the Linux side. hostnames may be nil (e.g. `proxy setup` run outside a
// project), in which case the guidance points at `proxy up` for the exact line.
func maybePrintWSLWindowsAccess(hostnames []string) {
	if !system.IsWSL() {
		return
	}

	caPath, err := proxy.CACertPath()
	if err != nil {
		return
	}

	printWSLGuidance(proxy.WSLWindowsAccessGuidance(caPath, hostnames))
}

// infraParams gathers the inputs proxy.PrepareInfra needs.
func (e *proxyEnvironment) infraParams() proxy.InfraParams {
	return proxy.InfraParams{
		CanonicalRoot: e.canonicalRoot,
		Hostname:      e.hostname,
		BaseDomain:    e.baseDomain,
		ConfigPath:    e.configPath,
	}
}

// composeOptions returns the compose options for this project in proxy mode
// (no host ports, joined to the shared proxy network with Traefik labels, CA
// mounted). up and dev-bootstrap use it to (re)generate compose.yaml directly,
// since they know the project is proxied even before its config records the
// hostname. APP_URL is not set here — proxy up writes it into .env.local before
// the container starts, keeping the file the single source of truth.
func (e *proxyEnvironment) composeOptions() *dockerpkg.ComposeOptions {
	opts := dockerpkg.ComposeOptionsFromConfig(e.cfg)
	if opts == nil {
		opts = &dockerpkg.ComposeOptions{}
	}

	caPath, _ := proxy.CACertPath()
	opts.Proxy = &dockerpkg.ProxyOptions{
		Hostname:       e.hostname,
		NetworkName:    proxy.NetworkName,
		CAPath:         caPath,
		AdminWatchPort: extension.AdminDevServerPort(e.projectRoot),
	}

	return opts
}

// bootstrapInfra sets up the shared proxy for this project and registers it,
// without starting or installing the shop — a proxy-mode `project dev` starts
// the environment and installs the shop itself. It seeds APP_URL in .env.local
// so that install uses the proxy hostname. Safe to call repeatedly.
func (e *proxyEnvironment) bootstrapInfra(ctx context.Context) error {
	reg, err := proxy.LoadRegistry()
	if err != nil {
		return err
	}

	if _, err := proxy.PrepareInfra(ctx, e.infraParams(), reg); err != nil {
		return err
	}

	// Seed APP_URL before the container starts so it boots on the proxy hostname
	// (.env.local is the single source of truth now that APP_URL is not pinned as
	// a container env var). Only when unset, so a value the user edited by hand
	// is left untouched.
	if envfile.ReadEnvVar(e.envLocalPath(), "APP_URL") == "" {
		if err := envfile.UpsertEnvVar(e.envLocalPath(), "APP_URL", "https://"+e.hostname); err != nil {
			return fmt.Errorf("setting APP_URL in .env.local: %w", err)
		}
	}

	// A born-proxy project has no PreviousAppURL/PreviousConfig (its configured
	// url is the hostname, so `proxy down` must not "restore" a port mode it
	// never had). But bootstrapInfra also runs on every `project dev` of a
	// project that opted in via `proxy up`, so it must PRESERVE that entry's
	// recorded restore state instead of overwriting it — otherwise a later
	// `proxy down` could no longer revert it.
	entry := proxy.ProjectEntry{
		ProjectRoot:  e.canonicalRoot,
		Hostname:     e.hostname,
		RegisteredAt: time.Now(),
	}
	if existing, found := reg.Find(e.canonicalRoot); found {
		entry.PreviousAppURL = existing.PreviousAppURL
		entry.PreviousConfig = existing.PreviousConfig
		entry.RegisteredAt = existing.RegisteredAt
	}
	reg.Upsert(entry)
	if err := reg.Save(); err != nil {
		return err
	}

	// Best-effort: makes the shop reachable at its own hostname from inside its
	// containers. Traefik is up by now.
	_ = proxy.ReconcileHostnames(ctx, reg.Hostnames())

	return nil
}

// ensureHostnameResolves checks whether the project's hostname will actually
// resolve to 127.0.0.1 and, when automatic wildcard DNS cannot cover it,
// explains why and shows the manual /etc/hosts line as the last resort. It
// never edits /etc/hosts itself.
func (e *proxyEnvironment) ensureHostnameResolves(ctx context.Context) {
	underBaseDomain := strings.HasSuffix(e.hostname, "."+e.baseDomain)

	if underBaseDomain && proxy.SupportsWildcardDNS(ctx) {
		if status := proxy.CheckResolverConfigured(e.baseDomain); !status.Configured {
			fmt.Println(tui.DimText.Render("  ") + tui.BoldText.Render(e.hostname) + tui.DimText.Render(" does not resolve yet. Run ") + tui.BoldText.Render("shopware-cli project proxy setup") + tui.DimText.Render(" once (needs sudo)."))
			fmt.Println()
		}
		return
	}

	if hostsFileContains(e.hostname) {
		return
	}

	if underBaseDomain {
		// Wildcard DNS is impossible on this system (Linux without
		// systemd-resolved), so the hostname needs a manual entry.
		fmt.Println(tui.DimText.Render("  Automatic DNS is not possible on this system: it does not run systemd-resolved,"))
		fmt.Println(tui.DimText.Render("  which shopware-cli needs to send *." + e.baseDomain + " lookups to its local DNS server."))
		fmt.Println(tui.DimText.Render("  Run ") + tui.BoldText.Render("shopware-cli project proxy setup") + tui.DimText.Render(" to see how to enable it."))
	} else {
		fmt.Println(tui.DimText.Render("  ") + tui.BoldText.Render(e.hostname) + tui.DimText.Render(" is outside *."+e.baseDomain+", so the automatic wildcard DNS does not cover it."))
	}
	fmt.Println(tui.DimText.Render("  As a last resort, add this line to /etc/hosts yourself (needs sudo):"))
	fmt.Println(tui.BoldText.Render("    127.0.0.1 " + e.hostname))
	fmt.Println()
}

// hostsFileContains reports whether /etc/hosts already mentions hostname, so
// the manual-entry guidance is not repeated once the user followed it.
func hostsFileContains(hostname string) bool {
	content, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}

		if slices.Contains(strings.Fields(line), hostname) {
			return true
		}
	}

	return false
}

// envLocalPath returns the project's .env.local file path.
func (e *proxyEnvironment) envLocalPath() string {
	return filepath.Join(e.projectRoot, ".env.local")
}

// proxyHostname returns the shop's proxy hostname if the project is
// registered with the shared proxy, or "" otherwise.
func proxyHostname(projectRoot string) string {
	reg, err := proxy.LoadRegistry()
	if err != nil {
		return ""
	}

	if entry, found := reg.Find(proxy.CanonicalProjectRoot(projectRoot)); found {
		return entry.Hostname
	}

	return ""
}

// switchProjectConfigURLs points the url keys in .shopware-project.yml at
// the proxy — the dev TUI and the admin API client resolve the shop URL from
// them — and returns the pre-proxy state for the registry, so down can
// restore the file exactly. On re-registration the state remembered by the
// first registration is kept. Projects without a config file return nil.
func (e *proxyEnvironment) switchProjectConfigURLs(reg proxy.Registry, proxyURL string) *shop.ConfigURLState {
	previous, alreadyManaged := previousConfigState(reg, e.canonicalRoot)
	if !alreadyManaged {
		state, err := shop.ReadProjectURLState(e.configPath, environmentName)
		if err != nil {
			fmt.Println(tui.RedText.Render("  Could not read the project config: " + err.Error()))
			return nil
		}
		if !state.HasFile {
			return nil
		}
		previous = &state
	}

	if err := shop.SetProjectURL(e.configPath, environmentName, proxyURL); err != nil {
		fmt.Println(tui.RedText.Render("  Could not update the url in the project config: " + err.Error()))
		if !alreadyManaged {
			return nil
		}
	}

	return previous
}

// previousConfigState returns the config state remembered by an existing
// registration, if any.
func previousConfigState(reg proxy.Registry, canonicalRoot string) (*shop.ConfigURLState, bool) {
	if old, found := reg.Find(canonicalRoot); found && old.PreviousConfig != nil {
		return old.PreviousConfig, true
	}

	return nil, false
}

// repointSalesChannel updates an installed shop's sales channel domain to
// toURL. Every URL in fromURLs is tried, since the domain may still carry an
// older value (e.g. the http:// variant of the proxy hostname from a previous
// registration). APP_URL in .env.local is set separately, before the container
// starts.
func (e *proxyEnvironment) repointSalesChannel(ctx context.Context, fromURLs []string, toURL string) error {
	// Repoint the sales channel domain with one direct UPDATE, which works on
	// every Shopware version (the sales-channel:replace:url console command is
	// 6.7+ only). The database is only reachable while the stack runs; when it
	// is down — e.g. deregistering an already-stopped project — opening the
	// connection fails and the caller surfaces a "restore once the shop runs"
	// hint. A not-yet-installed shop (no sales_channel_domain table) is a no-op:
	// the installer seeds the domain from APP_URL later.
	dbConn, err := e.executor.DatabaseConnection(ctx)
	if err != nil {
		if shopNotInstalled(err.Error()) {
			return nil
		}
		return fmt.Errorf("connecting to the database: %w", err)
	}

	conn, cleanup, err := dbConn.Open(ctx)
	if err != nil {
		if shopNotInstalled(err.Error()) {
			return nil
		}
		return fmt.Errorf("connecting to the database: %w", err)
	}
	defer cleanup()

	for _, fromURL := range fromURLs {
		if fromURL == toURL {
			continue
		}

		if _, err := conn.ExecContext(ctx, "UPDATE sales_channel_domain SET url = ? WHERE url = ?", toURL, fromURL); err != nil {
			if shopNotInstalled(err.Error()) {
				return nil
			}
			return fmt.Errorf("repointing sales channel url: %w", err)
		}
	}

	return nil
}

// shopNotInstalled reports whether console output indicates the shop has no
// installed database yet (no sales channels, missing tables/database), as
// opposed to a real failure. Such shops are compiled/seeded by the installer
// later, so URL-dependent steps can be skipped for them.
func shopNotInstalled(output string) bool {
	return strings.Contains(output, "No sales channels found") ||
		strings.Contains(output, "doesn't exist") ||
		strings.Contains(output, "Unknown database")
}

// recompileTheme rebuilds the storefront theme so the absolute asset URLs it
// bakes in at compile time (notably the JS import map) match the shop's new
// proxy URL. It is best-effort: a not-yet-installed shop has no theme to
// compile (the installer does it later, with the correct URL), so that is
// skipped quietly and only unexpected failures are surfaced without aborting.
// recompileTheme re-runs theme:compile after the shop's URL changed. It is
// needed because the storefront's ES module import map (Shopware 6.7+) is built
// at compile time and stored verbatim in theme_runtime_config, not resolved per
// request: ThemeCompiler builds each entry with the "asset" package
// (FallbackUrlPackage), whose URL falls back to the absolute APP_URL when there
// is no request — and theme:compile runs on the CLI, so it always bakes the
// absolute APP_URL. Switching the shop to its proxy hostname therefore leaves
// the stored map pointing at the old host until the theme is recompiled, and
// the browser fails to load the shopware core module from the stale URL.
// (Assets rendered via asset() in Twig are request-relative and unaffected;
// only the compiled-and-stored import map is.)
func (e *proxyEnvironment) recompileTheme(ctx context.Context) {
	err := runStep(ctx, "Updating storefront theme for the new URL...", func(ctx context.Context) error {
		if out, err := e.executor.ConsoleCommand(ctx, "theme:compile").CombinedOutput(); err != nil && !shopNotInstalled(string(out)) {
			return fmt.Errorf("%w\n%s", err, out)
		}
		return nil
	})
	if err != nil {
		fmt.Println(tui.RedText.Render("  Could not update the storefront theme (continuing): " + err.Error()))
	}
}

// down deregisters the project and stops it. hintTeardown controls whether
// the "run teardown" nudge is shown when no projects remain — teardown itself
// suppresses it.
func (e *proxyEnvironment) down(ctx context.Context, hintTeardown bool) error {
	reg, err := proxy.LoadRegistry()
	if err != nil {
		return err
	}

	entry, registered := reg.Find(e.canonicalRoot)

	if !registered {
		// Nothing was registered for this project, so there is nothing to
		// deregister and no reason to stop its environment. Still regenerate
		// compose.yaml in plain fixed-port mode to heal a proxy compose file
		// left by a partially-failed `up`, then report honestly instead of
		// claiming a deregistration.
		if err := dockerpkg.WriteComposeFile(e.projectRoot, dockerpkg.ComposeOptionsFromConfig(e.cfg)); err != nil {
			return err
		}
		fmt.Println(tui.DimText.Render("  ") + tui.BoldText.Render(e.hostname) + tui.DimText.Render(" is not registered with the shared proxy — nothing to deregister."))
		fmt.Println()
		return nil
	}

	// Only revert the shop's URLs when there is a genuine pre-proxy state to
	// return to — a port project that opted in via `proxy up`. A project
	// created with local domains has no port mode to restore (its identity is
	// the proxy hostname), so its URLs are left as they are; a later
	// `project dev` simply re-bootstraps the proxy.
	if registered && entry.PreviousAppURL != "" {
		if err := envfile.UpsertEnvVar(e.envLocalPath(), "APP_URL", entry.PreviousAppURL); err != nil {
			fmt.Println(tui.RedText.Render("  Could not restore APP_URL in .env.local: " + err.Error()))
		}
		if err := e.repointSalesChannel(ctx, []string{"https://" + e.hostname, "http://" + e.hostname}, entry.PreviousAppURL); err != nil {
			fmt.Println(tui.RedText.Render("  Could not restore the sales channel domain: " + err.Error()))
			fmt.Println(tui.DimText.Render("  Restore the sales channel domain to ") + tui.BoldText.Render(entry.PreviousAppURL) + tui.DimText.Render(" once the shop runs."))
		}
	}

	// Restore the url keys in .shopware-project.yml to their pre-proxy state.
	if registered && entry.PreviousConfig != nil {
		if err := shop.RestoreProjectURL(e.configPath, environmentName, *entry.PreviousConfig); err != nil {
			fmt.Println(tui.RedText.Render("  Could not restore the url in the project config: " + err.Error()))
		}
	}

	// Regenerate compose.yaml in plain fixed-port mode, reverting the project
	// out of proxy mode.
	if err := dockerpkg.WriteComposeFile(e.projectRoot, dockerpkg.ComposeOptionsFromConfig(e.cfg)); err != nil {
		return err
	}

	err = runStep(ctx, fmt.Sprintf("Stopping %s...", e.hostname), func(ctx context.Context) error {
		return e.executor.StopEnvironment(ctx)
	})
	if err != nil {
		return fmt.Errorf("stopping environment: %w", err)
	}

	if reg.Remove(e.canonicalRoot) {
		if err := reg.Save(); err != nil {
			return err
		}

		// Drop this hostname from the proxy's in-container aliases.
		if err := proxy.ReconcileHostnames(ctx, reg.Hostnames()); err != nil {
			fmt.Println(tui.RedText.Render("  Could not update in-container hostnames: " + err.Error()))
		}
	}

	// A manually added /etc/hosts line stays the user's responsibility; just
	// remind them it can go now.
	if hostsFileContains(e.hostname) {
		fmt.Println(tui.DimText.Render("  You can now remove the line for ") + tui.BoldText.Render(e.hostname) + tui.DimText.Render(" from /etc/hosts."))
	}

	fmt.Println(tui.GreenText.Bold(true).Render("  ✓ Deregistered "+e.hostname) + tui.DimText.Render("  "+e.projectRoot))
	fmt.Println()

	if hintTeardown && len(reg.Projects) == 0 {
		fmt.Println(tui.DimText.Render("  No other projects are registered. Run ") + tui.BoldText.Render("shopware-cli project proxy teardown") + tui.DimText.Render(" to stop the shared proxy too."))
		fmt.Println()
	}

	return nil
}

func init() {
	projectRootCmd.AddCommand(projectProxyCmd)
}
