package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
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
    127.0.0.1 via a small DNS server embedded in shopware-cli
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

		fmt.Println(tui.DimText.Render("  Proxy domain: ") + tui.BoldText.Render(baseDomain))
		if baseDomain != proxy.DefaultDomain {
			fmt.Println(tui.DimText.Render("  This custom domain is stored machine-wide. Reset it with --domain " + proxy.DefaultDomain))
		}
		fmt.Println()

		// DNS resolution for *.<domain>. The resolver is configured before
		// anything else is touched: when this fails (blocked sudo), a
		// pending domain change is simply not committed and the machine
		// keeps its previous, working domain.
		if status := proxy.CheckResolverConfigured(baseDomain); status.Configured {
			fmt.Println(tui.GreenText.Bold(true).Render("  ✓ DNS is already configured"))
			fmt.Println(tui.DimText.Render("  " + status.Detail))
		} else if err := proxy.ConfigureResolver(ctx, baseDomain); err != nil {
			if errors.Is(err, proxy.ErrNoSystemdResolved) {
				printGuidance(proxy.NoSystemdResolvedGuidance(baseDomain))
			} else {
				printGuidance(proxy.ResolverBlockedGuidance(baseDomain))
				if change != nil {
					fmt.Println()
					fmt.Println(tui.DimText.Render("  The proxy domain was not changed, it is still ") + tui.BoldText.Render(change.previous))
				}
				return err
			}
		} else {
			fmt.Println(tui.GreenText.Bold(true).Render("  ✓ DNS configured"))
			fmt.Println(tui.DimText.Render("  Every *." + baseDomain + " hostname now resolves to 127.0.0.1."))
		}

		if err := proxy.EnsureDNSServerRunning(baseDomain); err != nil {
			return fmt.Errorf("starting DNS server: %w", err)
		}

		if change != nil {
			if err := change.commit(ctx); err != nil {
				return err
			}
		}
		fmt.Println()

		// HTTPS trust.
		caPath, err := proxy.CACertPath()
		if err != nil {
			return fmt.Errorf("preparing certificate authority: %w", err)
		}

		if skipTrust, _ := cmd.Flags().GetBool("skip-trust"); skipTrust {
			fmt.Println(tui.DimText.Render("  Skipping trust store installation (--skip-trust)."))
			fmt.Println(tui.DimText.Render("  Browsers will show a security warning for the proxy's HTTPS pages (you can click through it)."))
			fmt.Println(tui.DimText.Render("  To get rid of the warning later, run this command (or ask your IT team to):"))
			fmt.Println(tui.DimText.Render("    " + proxy.TrustInstructions(caPath)))
			fmt.Println(tui.DimText.Render("  Firefox users can instead import the certificate without administrator rights:"))
			fmt.Println(tui.DimText.Render("    Settings > Privacy & Security > Certificates > View Certificates > Import: " + caPath))
		} else {
			summary, err := proxy.InstallTrust(ctx, caPath)
			if err != nil {
				printGuidance(proxy.TrustBlockedGuidance(caPath))
				return err
			}

			fmt.Println(tui.GreenText.Bold(true).Render("  ✓ HTTPS certificates are trusted"))
			fmt.Println(tui.DimText.Render("  " + summary))
		}
		fmt.Println()

		// Start the shared infrastructure and prove the whole chain works.
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

		fmt.Println(tui.BoldText.Render("  Verifying the setup:"))
		if !runProxyVerification(ctx, baseDomain) {
			return ErrProxyVerificationFailed
		}

		return nil
	},
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

// domainChange is a validated but not yet persisted --domain override. It is
// committed only after the new domain's DNS resolution is in place, so a
// failed setup (e.g. blocked sudo) leaves the previous domain fully working.
type domainChange struct {
	previous  string
	requested string
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

var projectProxyTeardownCmd = &cobra.Command{
	Use:          "teardown",
	SilenceUsage: true,
	Short:        "Deregister every project and stop the shared proxy and DNS server",
	Long: `Runs "project proxy down" for every registered project (stopping it and
restoring its previous URL), then stops the shared Traefik container and the
embedded DNS server. The one-time OS setup (DNS resolver, trusted CA) is kept.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		reg, err := proxy.LoadRegistry()
		if err != nil {
			return err
		}

		if len(reg.Projects) > 0 {
			if confirmed, err := confirmTeardown(cmd, reg); err != nil || !confirmed {
				return err
			}
		}

		for _, entry := range reg.Projects {
			env, err := newProxyEnvironmentForRoot(ctx, entry.ProjectRoot, filepath.Join(entry.ProjectRoot, ".shopware-project.yml"))
			if err == nil {
				err = env.down(ctx, false)
			}
			if err != nil {
				fmt.Println(tui.RedText.Render(fmt.Sprintf("  Could not deregister %s: %s", entry.Hostname, err)))
			}
		}

		if err := proxy.StopTraefik(ctx); err != nil {
			return err
		}

		if err := proxy.StopDNSServer(); err != nil {
			return err
		}

		fmt.Println(tui.GreenText.Bold(true).Render("  ✓ Shared proxy and DNS server stopped"))

		return nil
	},
}

// confirmTeardown lists what teardown is about to do and asks the user to
// confirm, unless --force was passed. Without a terminal it requires --force.
func confirmTeardown(cmd *cobra.Command, reg proxy.Registry) (bool, error) {
	instances, err := proxy.RunningInstances(cmd.Context())
	if err != nil {
		instances = nil // proxy may already be gone; states then show as stopped
	}

	fmt.Println(tui.BoldText.Render("  Tearing down the shared proxy will:"))
	for _, entry := range reg.Projects {
		state := tui.RedText.Render("stopped")
		if projectIsRunning(entry, instances) {
			state = tui.GreenText.Render("running")
		}

		fmt.Printf("    - deregister %s (%s) and restore its previous URL\n", tui.BoldText.Render(entry.Hostname), state)
	}
	fmt.Println("    - stop the shared Traefik container and the DNS server")
	fmt.Println()

	if force, _ := cmd.Flags().GetBool("force"); force {
		return true, nil
	}

	if !system.IsInteractionEnabled(cmd.Context()) || !isatty.IsTerminal(os.Stdin.Fd()) {
		return false, fmt.Errorf("teardown affects every registered project, run it with --force in non-interactive environments")
	}

	var confirmed bool
	if err := huh.NewConfirm().
		Title("Proceed with the teardown?").
		Value(&confirmed).
		Run(); err != nil {
		return false, err
	}

	if !confirmed {
		fmt.Println(tui.DimText.Render("  Teardown cancelled"))
	}

	return confirmed, nil
}

func init() {
	projectProxyCmd.AddCommand(projectProxySetupCmd)
	projectProxyCmd.AddCommand(projectProxyTeardownCmd)

	projectProxySetupCmd.Flags().Bool("skip-trust", false, "Skip installing the certificate authority into the trust stores")
	projectProxySetupCmd.Flags().String("domain", "", "Base domain for project hostnames (default "+proxy.DefaultDomain+", persisted machine-wide)")
	projectProxyTeardownCmd.Flags().Bool("force", false, "Tear down without asking for confirmation")
}
