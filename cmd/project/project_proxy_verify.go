package project

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/tui"
)

// ErrProxyVerificationFailed is returned when a proxy health check fails; it
// makes the command exit non-zero without an extra error message (the failed
// check and its hint were already printed).
var ErrProxyVerificationFailed = errors.New("proxy verification failed")

var projectProxyVerifyCmd = &cobra.Command{
	Use:           "verify",
	SilenceUsage:  true,
	SilenceErrors: true,
	Short:         "Check that proxied shops will be reachable on this machine",
	Long: `Verifies the whole shared-proxy chain bottom-up: Docker, the shared DNS
container, the operating system's hostname resolution, the Traefik container and
finally a trusted HTTPS request to the proxy's own health endpoint. The first
failing layer is reported with a hint how to fix it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := proxy.LoadSettings()
		if err != nil {
			return err
		}

		ready, notStarted := runProxyVerification(cmd.Context(), settings.BaseDomain())
		// "Not started yet" is not a fault — the machine may simply be set up but
		// idle (e.g. after a teardown). Only a real broken layer exits non-zero.
		if !ready && !notStarted {
			return ErrProxyVerificationFailed
		}

		return nil
	},
}

// runProxyVerification prints the outcome of every proxy health check. It
// returns ready (every check passed) and notStarted (the only failures are
// layers that are simply not running yet — the shared DNS or Traefik container,
// e.g. before the first `proxy up` or after a `teardown`), so callers can tell a
// broken setup from an idle one. Shared by `proxy verify` and the final step of
// `proxy setup`.
func runProxyVerification(ctx context.Context, baseDomain string) (ready, notStarted bool) {
	results := proxy.Verify(ctx, baseDomain)

	ready = true
	onlyPending := true
	for _, result := range results {
		if result.Err == nil {
			fmt.Println(tui.GreenText.Render("  ✓ ") + result.Name)
			continue
		}

		ready = false
		if result.Pending {
			// Calm marker: this layer just is not running yet, nothing is broken.
			fmt.Println(tui.DimText.Render("  ○ ") + result.Name + tui.DimText.Render("  (not started yet)"))
		} else {
			onlyPending = false
			fmt.Println(tui.RedText.Render("  ✗ ") + result.Name)
			fmt.Println(tui.DimText.Render("    " + result.Err.Error()))
		}

		for i, line := range strings.Split(result.Hint, "\n") {
			if i == 0 {
				fmt.Println(tui.DimText.Render("    Hint: " + line))
			} else {
				fmt.Println(tui.DimText.Render("          " + line))
			}
		}
	}

	notStarted = !ready && onlyPending

	fmt.Println()
	switch {
	case ready:
		fmt.Println(tui.GreenText.Bold(true).Render("  ✓ This machine is ready, run \"shopware-cli project proxy up\" in any shop"))
	case notStarted:
		fmt.Println(tui.BoldText.Render("  The shared proxy isn't running yet — nothing is wrong."))
		fmt.Println(tui.DimText.Render("  Run ") + tui.BoldText.Render("shopware-cli project proxy up") + tui.DimText.Render(" in a shop to start it (or ") + tui.BoldText.Render("proxy setup") + tui.DimText.Render(" for first-time machine setup)."))
	}

	return ready, notStarted
}

func init() {
	projectProxyCmd.AddCommand(projectProxyVerifyCmd)
}
