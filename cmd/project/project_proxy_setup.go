package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"charm.land/huh/v2/spinner"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectProxySetupCmd = &cobra.Command{
	Use:          "setup",
	SilenceUsage: true,
	Short:        "One-time machine setup for the shared proxy: DNS and HTTPS trust (needs sudo)",
	Long: `Performs the one-time machine setup for the shared proxy in a single sudo
ceremony:

  - configures the operating system to resolve every hostname under the proxy
    domain (default ` + proxy.DefaultDomain + `, changeable with --domain) to
    127.0.0.1 via a small DNS server (CoreDNS) run in a container
  - creates the local certificate authority (shared with mkcert) and installs
    it into the system and browser trust stores, so the HTTPS certificates the
    proxy serves are trusted

Both steps are idempotent; run it again anytime to repair the setup.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		baseDomain, change, err := resolveDomainFlag(cmd)
		if err != nil {
			return err
		}

		// Defense-in-depth: --domain is validated when set, but the stored value
		// flows straight into root-privileged resolver writes, so re-validate it
		// before touching the system.
		if err := proxy.ValidateDomain(baseDomain); err != nil {
			return fmt.Errorf("invalid proxy domain %q: %w", baseDomain, err)
		}

		fmt.Println(tui.DimText.Render("  Proxy domain: ") + tui.BoldText.Render(baseDomain))
		if baseDomain != proxy.DefaultDomain {
			fmt.Println(tui.DimText.Render("  This custom domain is stored machine-wide. Reset it with --domain " + proxy.DefaultDomain))
		}
		fmt.Println()

		skipTrust, _ := cmd.Flags().GetBool("skip-trust")

		caPath, err := proxy.CACertPath()
		if err != nil {
			return fmt.Errorf("preparing certificate authority: %w", err)
		}

		// The two system-touching steps — pointing the OS resolver at the DNS
		// server and trusting the local CA — both need sudo/admin, so they
		// share one choice: let shopware-cli do it, or print the steps for the
		// user to run.
		automatic, err := chooseAutomaticSetup(ctx, baseDomain, !skipTrust)
		if err != nil {
			return err
		}

		if automatic {
			// Configure the resolver before anything else is touched: when this
			// fails (blocked sudo), a pending domain change is not committed and
			// the machine keeps its previous, working domain.
			if err := configureResolverAutomatically(ctx, baseDomain, change); err != nil {
				return err
			}
		} else {
			printManualSetup(proxy.ManualSetupInstructions(baseDomain, caPath, !skipTrust))
		}

		// The DNS responder, certificate and Traefik run in containers and need
		// no elevated rights, so they come up the same way in both modes.
		if err := proxy.EnsureDNSContainerRunning(ctx, baseDomain); err != nil {
			return fmt.Errorf("starting DNS server: %w", err)
		}

		if change != nil {
			if automatic {
				if err := change.commit(ctx); err != nil {
					return err
				}
			} else if err := change.persist(); err != nil {
				return err
			} else {
				fmt.Println(tui.DimText.Render("  Proxy domain set to ") + tui.BoldText.Render(change.requested))
			}
		}
		fmt.Println()

		if automatic {
			if err := installTrustAutomatically(ctx, caPath, skipTrust); err != nil {
				return err
			}
			fmt.Println()
		}

		// Start the shared infrastructure.
		dir, err := proxy.StateDir()
		if err != nil {
			return err
		}

		certInfo, err := proxy.EnsureCertificate(dir, proxy.CertHosts(baseDomain, nil))
		if err != nil {
			return err
		}

		if err := proxy.EnsureTraefikRunning(ctx, baseDomain); err != nil {
			return err
		}

		// A regenerated certificate (e.g. after a domain change) is only
		// served after a restart.
		if certInfo.Changed {
			if err := proxy.RestartTraefik(ctx); err != nil {
				return err
			}
		}

		// In automatic mode the whole chain should already work, so prove it. In
		// manual mode the user still has to run the printed steps, so a strict
		// check would fail — point them at `proxy verify` for afterwards instead.
		if automatic {
			fmt.Println(tui.BoldText.Render("  Verifying the setup:"))
			// setup just started the DNS and Traefik containers, so here anything
			// short of fully ready (including "not started yet") is a real failure.
			if ready, _ := runProxyVerification(ctx, baseDomain); !ready {
				return ErrProxyVerificationFailed
			}
		} else {
			fmt.Println(tui.DimText.Render("  Once you have run the steps above, verify with ") + tui.BoldText.Render("shopware-cli project proxy verify"))
		}

		// Under WSL the setup only touched the Linux side; reaching shops from a
		// Windows browser needs manual steps. Use the current project's hostnames
		// when setup is run inside one, otherwise a generic pointer.
		maybePrintWSLWindowsAccess(setupProjectHostnames(ctx))

		return nil
	},
}

// setupProjectHostnames returns the proxy hostnames of the project in the
// current directory for the WSL guidance, or nil when `proxy setup` is not run
// inside a project (it is a machine-wide command).
func setupProjectHostnames(ctx context.Context) []string {
	if !system.IsWSL() {
		return nil
	}

	projectRoot, err := findClosestShopwareProject()
	if err != nil {
		return nil
	}

	env, err := newProxyEnvironmentForRoot(ctx, projectRoot, filepath.Join(projectRoot, ".shopware-project.yml"))
	if err != nil {
		return nil
	}

	return proxyBrowserHostnames(projectRoot, env.hostname)
}

// runInlineProxySetup performs the one-time, sudo-requiring machine setup —
// pointing the OS resolver at the shared DNS container for baseDomain and
// installing the proxy CA into the trust stores — as offered inline by
// `project create` when a user opts into local domains. It is the core of the
// `proxy setup` command without the domain-change and verification machinery
// (a later `project dev` starts Traefik and serves the shop). It prints its own
// progress and guidance; the returned error only signals that a step was
// blocked, so the caller can fall back to a "run proxy setup later" hint.
func runInlineProxySetup(ctx context.Context, baseDomain string) error {
	// Defense-in-depth: the stored domain flows into root-privileged resolver
	// writes, so validate it before touching the system.
	if err := proxy.ValidateDomain(baseDomain); err != nil {
		return fmt.Errorf("invalid proxy domain %q: %w", baseDomain, err)
	}

	caPath, err := proxy.CACertPath()
	if err != nil {
		return fmt.Errorf("preparing certificate authority: %w", err)
	}

	automatic, err := chooseAutomaticSetup(ctx, baseDomain, true)
	if err != nil {
		return err
	}

	if !automatic {
		printManualSetup(proxy.ManualSetupInstructions(baseDomain, caPath, true))
		// The DNS responder still starts (no sudo); the shop's `project dev`
		// brings up Traefik and the certificate afterwards.
		if err := proxy.EnsureDNSContainerRunning(ctx, baseDomain); err != nil {
			return fmt.Errorf("starting DNS server: %w", err)
		}
		return nil
	}

	if err := configureResolverAutomatically(ctx, baseDomain, nil); err != nil {
		return err
	}

	if err := proxy.EnsureDNSContainerRunning(ctx, baseDomain); err != nil {
		return fmt.Errorf("starting DNS server: %w", err)
	}

	return installTrustAutomatically(ctx, caPath, false)
}

// chooseAutomaticSetup explains the one-time system changes (OS resolver + CA
// trust) and asks whether shopware-cli should apply them itself (needs
// sudo/admin) or just print the steps. Non-interactively it never runs sudo
// unprompted: it returns false so agents and CI get the instructions instead.
func chooseAutomaticSetup(ctx context.Context, baseDomain string, includeTrust bool) (bool, error) {
	fmt.Println(tui.BoldText.Render("  Reaching your shops at trusted HTTPS hostnames needs a one-time change to this machine:"))
	fmt.Println(tui.DimText.Render("    - resolve *." + baseDomain + " to 127.0.0.1 (writes an OS resolver file, needs sudo)"))
	if includeTrust {
		fmt.Println(tui.DimText.Render("    - trust the local HTTPS certificate (installs a local CA, needs sudo)"))
	}
	fmt.Println()

	if !system.IsInteractionEnabled(ctx) || !isatty.IsTerminal(os.Stdin.Fd()) {
		// Never sudo without being asked: fall back to printing the steps.
		return false, nil
	}

	doItForMe := true
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[bool]().
			Title("How should this be set up?").
			Options(
				huh.NewOption("Set it up for me (you may be asked for your password)", true),
				huh.NewOption("I'll do it myself (show me the steps)", false),
			).
			Value(&doItForMe),
	)).WithTheme(tui.ShopwareTheme())
	if err := form.Run(); err != nil {
		return false, err
	}

	return doItForMe, nil
}

// configureResolverAutomatically points the OS resolver at the DNS server via
// sudo, printing progress and guidance. A missing systemd-resolved is not
// fatal (the shop still works once the user adds a manual entry); a blocked
// sudo is returned so the caller stops.
func configureResolverAutomatically(ctx context.Context, baseDomain string, change *domainChange) error {
	if status := proxy.CheckResolverConfigured(baseDomain); status.Configured {
		fmt.Println(tui.GreenText.Bold(true).Render("  ✓ DNS is already configured"))
		fmt.Println(tui.DimText.Render("  " + status.Detail))
		return nil
	}

	if err := proxy.ConfigureResolver(ctx, baseDomain); err != nil {
		if errors.Is(err, proxy.ErrNoSystemdResolved) {
			printGuidance(proxy.NoSystemdResolvedGuidance(baseDomain))
			return nil
		}

		printGuidance(proxy.ResolverBlockedGuidance(baseDomain))
		if change != nil {
			fmt.Println()
			fmt.Println(tui.DimText.Render("  The proxy domain was not changed, it is still ") + tui.BoldText.Render(change.previous))
		}
		return err
	}

	fmt.Println(tui.GreenText.Bold(true).Render("  ✓ DNS configured"))
	fmt.Println(tui.DimText.Render("  Every *." + baseDomain + " hostname now resolves to 127.0.0.1."))
	return nil
}

// installTrustAutomatically installs the local CA into the trust stores, or
// prints the manual command and the browser-warning caveat when trust is
// skipped with --skip-trust.
func installTrustAutomatically(ctx context.Context, caPath string, skipTrust bool) error {
	if skipTrust {
		fmt.Println(tui.DimText.Render("  Skipping trust store installation (--skip-trust)."))
		fmt.Println(tui.DimText.Render("  Browsers will show a security warning for the proxy's HTTPS pages (you can click through it)."))
		fmt.Println(tui.DimText.Render("  To get rid of the warning later, run this command (or ask your IT team to):"))
		fmt.Println(tui.DimText.Render("    " + proxy.TrustInstructions(caPath)))
		fmt.Println(tui.DimText.Render("  Firefox users can instead import the certificate without administrator rights:"))
		fmt.Println(tui.DimText.Render("    Settings > Privacy & Security > Certificates > View Certificates > Import: " + caPath))
		return nil
	}

	summary, err := proxy.InstallTrust(ctx, caPath)
	if err != nil {
		printGuidance(proxy.TrustBlockedGuidance(caPath))
		return err
	}

	fmt.Println(tui.GreenText.Bold(true).Render("  ✓ HTTPS certificates are trusted"))
	fmt.Println(tui.DimText.Render("  " + summary))
	return nil
}

// printManualSetup renders the do-it-yourself steps with a neutral (non-error)
// bold headline and dimmed body.
func printManualSetup(instructions string) {
	fmt.Println(tui.BoldText.Render("  Set it up yourself with these steps:"))
	fmt.Println()
	for _, line := range strings.Split(instructions, "\n") {
		fmt.Println(tui.DimText.Render("  " + line))
	}
	fmt.Println()
}

// runStep runs a potentially slow action, showing a spinner with the given
// title only when running interactively. Without a terminal (CI, piped
// output) the bubbletea spinner cannot run and would skip the action, so the
// action is executed directly instead.
func runStep(ctx context.Context, title string, action func(context.Context) error) error {
	if !system.IsInteractionEnabled(ctx) || !isatty.IsTerminal(os.Stdout.Fd()) {
		return action(ctx)
	}

	return spinner.New().Title(title).Context(ctx).ActionWithErr(action).Run()
}

// printGuidance renders a multi-line help text: the first line as the red
// headline, the rest dimmed. Guidance texts live in the proxy package (where
// they are unit-tested) and are self-contained.
func printGuidance(guidance string) {
	for i, line := range strings.Split(guidance, "\n") {
		if i == 0 {
			fmt.Println(tui.RedText.Render("  " + line))
		} else {
			fmt.Println(tui.DimText.Render("  " + line))
		}
	}
}

// printWSLGuidance renders informational (non-error) WSL guidance: a bold
// headline followed by dimmed body and commands, framed by blank lines.
func printWSLGuidance(guidance string) {
	fmt.Println()
	for i, line := range strings.Split(strings.TrimRight(guidance, "\n"), "\n") {
		if i == 0 {
			fmt.Println(tui.BoldText.Render("  " + line))
		} else {
			fmt.Println(tui.DimText.Render("  " + line))
		}
	}
	fmt.Println()
}

// domainChange is a validated but not yet persisted --domain override. It is
// committed only after the new domain's DNS resolution is in place, so a
// failed setup (e.g. blocked sudo) leaves the previous domain fully working.
type domainChange struct {
	previous  string
	requested string
}

// persist saves the new domain machine-wide without touching the OS resolver,
// used in manual mode where the user applies the resolver change themselves.
func (c *domainChange) persist() error {
	return proxy.SaveSettings(proxy.Settings{Domain: c.requested})
}

// commit persists the new domain and removes the previous domain's resolver
// configuration (best-effort; it is harmless but useless once the settings
// point elsewhere).
func (c *domainChange) commit(ctx context.Context) error {
	if err := proxy.SaveSettings(proxy.Settings{Domain: c.requested}); err != nil {
		return err
	}

	if err := proxy.UnconfigureResolver(ctx, c.previous); err != nil {
		fmt.Println(tui.RedText.Render(fmt.Sprintf("  Could not remove the resolver configuration for %s: %s", c.previous, err)))
	}

	fmt.Println(tui.DimText.Render("  Proxy domain changed from ") + tui.BoldText.Render(c.previous) + tui.DimText.Render(" to ") + tui.BoldText.Render(c.requested))

	return nil
}

// resolveDomainFlag resolves the machine-wide proxy domain and validates a
// --domain override without any side effects. Changing the domain is refused
// while projects are registered, since their hostnames, certificates and
// URLs all embed it. A non-nil domainChange is returned for the caller to
// commit once the new domain provably works.
func resolveDomainFlag(cmd *cobra.Command) (string, *domainChange, error) {
	settings, err := proxy.LoadSettings()
	if err != nil {
		return "", nil, err
	}

	requested, _ := cmd.Flags().GetString("domain")
	if requested == "" || requested == settings.BaseDomain() {
		return settings.BaseDomain(), nil, nil
	}

	if err := proxy.ValidateDomain(requested); err != nil {
		return "", nil, err
	}

	reg, err := proxy.LoadRegistry()
	if err != nil {
		return "", nil, err
	}

	if len(reg.Projects) > 0 {
		return "", nil, fmt.Errorf("cannot change the proxy domain while %d project(s) are registered, run \"shopware-cli project proxy teardown\" first", len(reg.Projects))
	}

	return requested, &domainChange{previous: settings.BaseDomain(), requested: requested}, nil
}

func init() {
	projectProxyCmd.AddCommand(projectProxySetupCmd)

	projectProxySetupCmd.Flags().Bool("skip-trust", false, "Skip installing the certificate authority into the trust stores")
	projectProxySetupCmd.Flags().String("domain", "", "Base domain for project hostnames (default "+proxy.DefaultDomain+", persisted machine-wide)")
}
