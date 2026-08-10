package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/internal/executor"
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

var projectProxyUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Register the current project with the shared proxy and start it",
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := newProxyEnvironment(cmd)
		if err != nil {
			return err
		}

		return env.up(cmd)
	},
}

var projectProxyDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Deregister the current project from the shared proxy and stop it",
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := newProxyEnvironment(cmd)
		if err != nil {
			return err
		}

		return env.down(cmd.Context(), true)
	},
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
		certInfo, infraErr = e.prepareProxyInfra(ctx, reg)
		return infraErr
	})
	if err != nil {
		return err
	}

	// The base compose.yaml stays in fixed-port mode; proxy mode is the
	// separate override (written by prepareProxyInfra) that docker compose
	// merges automatically, so `project dev` and manual `docker compose` keep
	// working in both modes without knowing about the proxy.
	if err := dockerpkg.WriteComposeFile(e.projectRoot, dockerpkg.ComposeOptionsFromConfig(e.cfg)); err != nil {
		return err
	}

	proxyURL := "https://" + e.hostname

	start := time.Now()
	err = runStep(ctx, "Starting development environment...", func(ctx context.Context) error {
		return e.executor.StartEnvironment(ctx)
	})
	if err != nil {
		return fmt.Errorf("starting environment: %w", err)
	}
	elapsed := time.Since(start).Round(time.Millisecond)

	// Point the application at its proxy hostname: APP_URL for env-driven
	// code paths and installs, the sales channel domain for installed shops.
	// Remember the previous APP_URL so "proxy down" can restore it.
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

	// Whether the shop's live URL actually changes decides if the storefront
	// theme needs a recompile: it bakes absolute asset URLs (the JS import map)
	// in at compile time, so only a changed URL requires paying that cost.
	urlChanged := envfile.ReadEnvVar(e.envLocalPath(), "APP_URL") != proxyURL

	if err := e.pointShopAt(ctx, []string{previousAppURL, "http://" + e.hostname}, proxyURL); err != nil {
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

// prepareProxyInfra brings up everything the shared proxy needs before a
// project's containers start: it checks the hostname is free, verifies compose
// supports resets, ensures the certificate, the shared Traefik container and
// the DNS container, and writes the compose override with APP_URL pinned (so PHP
// renders absolute URLs — e.g. the storefront import map — with the proxy
// hostname, not the stale image default). It returns the certificate info so
// callers can react to a freshly created CA. It neither starts nor registers
// the project: up() and a proxy-mode `project dev` layer their own
// start/registration on top. Safe to call repeatedly.
func (e *proxyEnvironment) prepareProxyInfra(ctx context.Context, reg proxy.Registry) (proxy.CertInfo, error) {
	if other, found := reg.FindByHostname(e.hostname, e.canonicalRoot); found {
		return proxy.CertInfo{}, fmt.Errorf("hostname %s is already registered to %s, set a different \"url\" in %s to disambiguate", e.hostname, other.ProjectRoot, projectConfigPath)
	}

	if err := proxy.EnsureComposeSupportsReset(ctx); err != nil {
		return proxy.CertInfo{}, err
	}

	certInfo, err := e.ensureCertificate(reg)
	if err != nil {
		return proxy.CertInfo{}, err
	}

	if err := proxy.EnsureTraefikRunning(ctx, e.baseDomain); err != nil {
		return proxy.CertInfo{}, err
	}
	// A regenerated certificate (e.g. new project wildcard SANs) is only served
	// after a restart.
	if certInfo.Changed {
		if err := proxy.RestartTraefik(ctx); err != nil {
			return proxy.CertInfo{}, err
		}
	}

	if err := proxy.EnsureDNSContainerRunning(ctx, e.baseDomain); err != nil {
		return proxy.CertInfo{}, fmt.Errorf("starting DNS server: %w", err)
	}

	if err := dockerpkg.WriteComposeOverride(e.projectRoot, &dockerpkg.ProxyOptions{
		Hostname:    e.hostname,
		NetworkName: proxy.NetworkName,
		CAPath:      certInfo.CAPath,
		AppURL:      "https://" + e.hostname,
	}); err != nil {
		return proxy.CertInfo{}, err
	}

	return certInfo, nil
}

// bootstrapInfra sets up the shared proxy for this project and registers it,
// without starting or installing the shop — a proxy-mode `project dev` starts
// the environment and installs the shop itself (the pinned APP_URL makes the
// install use the proxy hostname). Safe to call repeatedly.
func (e *proxyEnvironment) bootstrapInfra(ctx context.Context) error {
	reg, err := proxy.LoadRegistry()
	if err != nil {
		return err
	}

	if _, err := e.prepareProxyInfra(ctx, reg); err != nil {
		return err
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
func (e *proxyEnvironment) switchProjectConfigURLs(reg proxy.Registry, proxyURL string) *proxy.ConfigURLState {
	previous, alreadyManaged := previousConfigState(reg, e.canonicalRoot)
	if !alreadyManaged {
		state, err := proxy.ReadProjectConfigURLs(e.configPath, environmentName)
		if err != nil {
			fmt.Println(tui.RedText.Render("  Could not read the project config: " + err.Error()))
			return nil
		}
		if !state.HasFile {
			return nil
		}
		previous = &state
	}

	if err := proxy.SetProjectConfigURLs(e.configPath, environmentName, proxyURL); err != nil {
		fmt.Println(tui.RedText.Render("  Could not update the url in the project config: " + err.Error()))
		if !alreadyManaged {
			return nil
		}
	}

	return previous
}

// previousConfigState returns the config state remembered by an existing
// registration, if any.
func previousConfigState(reg proxy.Registry, canonicalRoot string) (*proxy.ConfigURLState, bool) {
	if old, found := reg.Find(canonicalRoot); found && old.PreviousConfig != nil {
		return old.PreviousConfig, true
	}

	return nil, false
}

// ensureCertificate makes sure the shared server certificate covers this
// project and every other registered one. TLS wildcards only match a single
// label, so each project contributes "*.<hostname>" for its service
// subdomains (mailer.<hostname>, adminer.<hostname>, ...).
func (e *proxyEnvironment) ensureCertificate(reg proxy.Registry) (proxy.CertInfo, error) {
	extraHosts := []string{e.hostname, "*." + e.hostname}
	for _, p := range reg.Projects {
		extraHosts = append(extraHosts, p.Hostname, "*."+p.Hostname)
	}

	dir, err := proxy.StateDir()
	if err != nil {
		return proxy.CertInfo{}, err
	}

	return proxy.EnsureCertificate(dir, proxy.CertHosts(e.baseDomain, extraHosts))
}

// pointShopAt switches the shop to toURL: APP_URL in .env.local and, for
// installed shops, the sales channel domain via the core
// sales-channel:replace:url console command. Every URL in fromURLs is tried,
// since the domain may still carry an older value (e.g. the http:// variant
// of the proxy hostname from a previous registration).
func (e *proxyEnvironment) pointShopAt(ctx context.Context, fromURLs []string, toURL string) error {
	if err := envfile.UpsertEnvVar(e.envLocalPath(), "APP_URL", toURL); err != nil {
		return err
	}

	// When the shop's containers are stopped (deregistering a stopped
	// project), compose exec is impossible; compose run starts the database
	// dependency and executes the command in a throwaway container instead.
	viaRun := false

	for _, fromURL := range fromURLs {
		if fromURL == toURL {
			continue
		}

		output, err := e.replaceSalesChannelURL(ctx, fromURL, toURL, viaRun)
		if err != nil && !viaRun && strings.Contains(string(output), "is not running") {
			viaRun = true
			output, err = e.replaceSalesChannelURL(ctx, fromURL, toURL, viaRun)
		}

		if err != nil {
			// No matching domain means the shop is not installed yet (the
			// installer seeds the domain from APP_URL), the domain was
			// changed manually, or this candidate simply is not the current
			// value; missing tables likewise mean a not-yet-installed shop.
			if shopNotInstalled(string(output)) {
				continue
			}

			return fmt.Errorf("replacing sales channel url: %w\n%s", err, output)
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

// replaceSalesChannelURL repoints the sales channel domain from fromURL to
// toURL. Shopware 6.7+ has the core sales-channel:replace:url command, run in
// the running web container or, when the project is stopped, in a temporary one
// (viaRun) whose database dependency docker compose starts. Shopware 6.6 and
// earlier have no clean equivalent (replace:url is absent and update:domain
// keeps the old scheme and port), so the domain row is rewritten with a direct
// SQL UPDATE in the database container instead.
func (e *proxyEnvironment) replaceSalesChannelURL(ctx context.Context, fromURL, toURL string, viaRun bool) ([]byte, error) {
	if !projectHasReplaceURLCommand(e.projectRoot) {
		return e.repointSalesChannelViaSQL(ctx, fromURL, toURL, viaRun)
	}

	if !viaRun {
		return e.executor.ConsoleCommand(ctx, "sales-channel:replace:url", fromURL, toURL).CombinedOutput()
	}

	cmd := exec.CommandContext(ctx, "docker", "compose", "run", "--rm", "-T", "web", "php", "bin/console", "sales-channel:replace:url", fromURL, toURL)
	cmd.Dir = e.projectRoot

	return cmd.CombinedOutput()
}

// proxyDBService is the database service name defined by
// internal/docker/compose.go (with root/root credentials and the "shopware"
// database), used for the 6.6 SQL fallback in replaceSalesChannelURL.
const proxyDBService = "database"

// repointSalesChannelViaSQL rewrites the sales channel domain URL directly in
// the database, the only reliable way on Shopware 6.6 (see
// replaceSalesChannelURL). It targets the running database container; a stopped
// stack (viaRun) has no server to talk to, so it is skipped quietly like a
// not-yet-installed shop.
func (e *proxyEnvironment) repointSalesChannelViaSQL(ctx context.Context, fromURL, toURL string, viaRun bool) ([]byte, error) {
	if viaRun {
		return nil, nil
	}

	// fromURL/toURL are controlled values (the proxy hostname and the previous
	// URL), but double any single quote defensively so a value can never break
	// out of the SQL string literal.
	esc := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	query := fmt.Sprintf("UPDATE sales_channel_domain SET url = '%s' WHERE url = '%s';", esc(toURL), esc(fromURL))

	cmd := exec.CommandContext(ctx, "docker", "compose", "exec", "-T", proxyDBService, "mariadb", "-uroot", "-proot", "shopware", "-e", query)
	cmd.Dir = e.projectRoot

	return cmd.CombinedOutput()
}

// replaceURLCommandMinVersion is the first Shopware release with the
// sales-channel:replace:url console command.
const replaceURLCommandMinVersion = "6.7.0.0"

// projectHasReplaceURLCommand reports whether the project's Shopware version
// ships sales-channel:replace:url (6.7+). Unknown or unparseable versions are
// treated as current (6.7+).
func projectHasReplaceURLCommand(projectRoot string) bool {
	lock, err := composer.ReadLock(filepath.Join(projectRoot, "composer.lock"))
	if err != nil {
		return true
	}

	floor := version.Must(version.NewVersion(replaceURLCommandMinVersion))
	for _, name := range []string{"shopware/core", "shopware/platform"} {
		pkg := lock.GetPackage(name)
		if pkg == nil {
			continue
		}
		v, err := version.NewVersion(strings.TrimPrefix(pkg.Version, "v"))
		if err != nil {
			return true
		}
		return v.GreaterThanOrEqual(floor)
	}

	return true
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
		// deregister and no reason to stop its environment. Still remove an
		// orphaned proxy override if one is present (a partially-failed `up`),
		// then report honestly instead of claiming a deregistration.
		if err := dockerpkg.RemoveComposeOverride(e.projectRoot); err != nil {
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
		if err := e.pointShopAt(ctx, []string{"https://" + e.hostname, "http://" + e.hostname}, entry.PreviousAppURL); err != nil {
			fmt.Println(tui.RedText.Render("  Could not restore the sales channel domain: " + err.Error()))
			if projectHasReplaceURLCommand(e.projectRoot) {
				fmt.Println(tui.DimText.Render("  Restore it manually once the shop runs: ") + tui.BoldText.Render(fmt.Sprintf("shopware-cli project console sales-channel:replace:url https://%s %s", e.hostname, entry.PreviousAppURL)))
			} else {
				fmt.Println(tui.DimText.Render("  Restore the sales channel domain to ") + tui.BoldText.Render(entry.PreviousAppURL) + tui.DimText.Render(" once the shop runs."))
			}
		}
	}

	// Restore the url keys in .shopware-project.yml to their pre-proxy state.
	if registered && entry.PreviousConfig != nil {
		if err := proxy.RestoreProjectConfigURLs(e.configPath, environmentName, *entry.PreviousConfig); err != nil {
			fmt.Println(tui.RedText.Render("  Could not restore the url in the project config: " + err.Error()))
		}
	}

	// Regenerating the base file also heals compose.yaml files that older
	// CLI versions wrote with the proxy config baked in.
	if err := dockerpkg.WriteComposeFile(e.projectRoot, dockerpkg.ComposeOptionsFromConfig(e.cfg)); err != nil {
		return err
	}

	if err := dockerpkg.RemoveComposeOverride(e.projectRoot); err != nil {
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
	projectProxyCmd.AddCommand(projectProxyUpCmd)
	projectProxyCmd.AddCommand(projectProxyDownCmd)
}
