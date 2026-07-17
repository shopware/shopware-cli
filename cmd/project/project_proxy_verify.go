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
	Long: `Verifies the whole shared-proxy chain bottom-up: Docker, the embedded DNS
server, the operating system's hostname resolution, the Traefik container and
finally a trusted HTTPS request to the proxy's own health endpoint. The first
failing layer is reported with a hint how to fix it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := proxy.LoadSettings()
		if err != nil {
			return err
		}

		if !runProxyVerification(cmd.Context(), settings.BaseDomain()) {
			return ErrProxyVerificationFailed
		}

		return nil
	},
}

// runProxyVerification prints the outcome of every proxy health check and
// reports whether all of them passed. Shared by `proxy verify` and the final
// step of `proxy setup`.
func runProxyVerification(ctx context.Context, baseDomain string) bool {
	results := proxy.Verify(ctx, baseDomain)

	ok := true
	for _, result := range results {
		if result.Err == nil {
			fmt.Println(tui.GreenText.Render("  ✓ ") + result.Name)
			continue
		}

		ok = false
		fmt.Println(tui.RedText.Render("  ✗ ") + result.Name)
		fmt.Println(tui.DimText.Render("    " + result.Err.Error()))

		for i, line := range strings.Split(result.Hint, "\n") {
			if i == 0 {
				fmt.Println(tui.DimText.Render("    Hint: " + line))
			} else {
				fmt.Println(tui.DimText.Render("          " + line))
			}
		}
	}

	if ok {
		fmt.Println()
		fmt.Println(tui.GreenText.Bold(true).Render("  ✓ This machine is ready, run \"shopware-cli project proxy up\" in any shop"))
	}

	return ok
}

func init() {
	projectProxyCmd.AddCommand(projectProxyVerifyCmd)
}
