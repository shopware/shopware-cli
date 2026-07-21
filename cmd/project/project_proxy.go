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
hostnames like http://my-shop.shopware.local to your local Shopware instances.
This allows running multiple instances in parallel without juggling ports.`,
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

	if err := system.ValidateProjectDependencies(cmd.Context(), true, nil, "manage the shared proxy", ""); err != nil {
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

	if other, found := reg.FindByHostname(e.hostname, e.canonicalRoot); found {
		return fmt.Errorf("hostname %s is already registered to %s, set a different \"url\" in %s to disambiguate", e.hostname, other.ProjectRoot, projectConfigPath)
	}

	if err := proxy.EnsureComposeSupportsReset(ctx); err != nil {
		return err
	}

	certInfo, err := e.ensureCertificate(reg)
	if err != nil {
		return err
	}

	err = runStep(ctx, "Starting shared proxy...", func(ctx context.Context) error {
		if err := proxy.EnsureTraefikRunning(ctx, e.baseDomain); err != nil {
			return err
		}

		// A regenerated certificate (e.g. new project wildcard SANs) is
		// only served after a restart.
		if certInfo.Changed {
			return proxy.RestartTraefik(ctx)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("starting shared proxy: %w", err)
	}

	if err := proxy.EnsureDNSServerRunning(e.baseDomain); err != nil {
		return fmt.Errorf("starting DNS server: %w", err)
	}

	// The base compose.yaml stays in fixed-port mode; proxy mode is a
	// separate override file docker compose merges automatically. This keeps
	// `project dev` and manual `docker compose` invocations working in both
	// modes without knowing about the proxy.
	if err := dockerpkg.WriteComposeFile(e.projectRoot, dockerpkg.ComposeOptionsFromConfig(e.cfg)); err != nil {
		return err
	}

	if err := dockerpkg.WriteComposeOverride(e.projectRoot, &dockerpkg.ProxyOptions{
		Hostname:    e.hostname,
		NetworkName: proxy.NetworkName,
		CAPath:      certInfo.CAPath,
	}); err != nil {
		return err
	}

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

	proxyURL := "https://" + e.hostname
	if err := e.pointShopAt(ctx, []string{previousAppURL, "http://" + e.hostname}, proxyURL); err != nil {
		fmt.Println(tui.RedText.Render("  Could not update the shop URL: " + err.Error()))
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
			outStr := string(output)
			if strings.Contains(outStr, "No sales channels found") || strings.Contains(outStr, "doesn't exist") || strings.Contains(outStr, "Unknown database") {
				continue
			}

			return fmt.Errorf("replacing sales channel url: %w\n%s", err, output)
		}
	}

	return nil
}

// replaceSalesChannelURL runs the core sales-channel:replace:url command,
// either in the running web container or, when the project is stopped, in a
// temporary one (viaRun) whose database dependency docker compose starts and
// the caller's environment stop cleans up again.
func (e *proxyEnvironment) replaceSalesChannelURL(ctx context.Context, fromURL, toURL string, viaRun bool) ([]byte, error) {
	if !viaRun {
		return e.executor.ConsoleCommand(ctx, "sales-channel:replace:url", fromURL, toURL).CombinedOutput()
	}

	cmd := exec.CommandContext(ctx, "docker", "compose", "run", "--rm", "-T", "web", "php", "bin/console", "sales-channel:replace:url", fromURL, toURL)
	cmd.Dir = e.projectRoot

	return cmd.CombinedOutput()
}

// down deregisters the project and stops it. hintTeardown controls whether
// the "run teardown" nudge is shown when no projects remain — teardown itself
// suppresses it.
func (e *proxyEnvironment) down(ctx context.Context, hintTeardown bool) error {
	reg, err := proxy.LoadRegistry()
	if err != nil {
		return err
	}

	// Point the shop back at its previous URL while the database is still
	// running.
	restoreURL := defaultShopURL
	entry, registered := reg.Find(e.canonicalRoot)
	if registered && entry.PreviousAppURL != "" {
		restoreURL = entry.PreviousAppURL
	}
	if err := e.pointShopAt(ctx, []string{"https://" + e.hostname, "http://" + e.hostname}, restoreURL); err != nil {
		fmt.Println(tui.RedText.Render("  Could not restore the sales channel domain: " + err.Error()))
		fmt.Println(tui.DimText.Render("  Restore it manually once the shop runs: ") + tui.BoldText.Render(fmt.Sprintf("shopware-cli project console sales-channel:replace:url https://%s %s", e.hostname, restoreURL)))
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

	fmt.Println(tui.GreenText.Bold(true).Render(fmt.Sprintf("  ✓ Deregistered %s", e.hostname)) + tui.DimText.Render("  "+e.projectRoot))
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
