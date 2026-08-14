package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectProxyTeardownCmd = &cobra.Command{
	Use:          "teardown",
	SilenceUsage: true,
	Short:        "Deregister every project and stop the shared proxy and DNS server",
	Long: `Runs "project proxy down" for every registered project (stopping it and
restoring its previous URL), then stops the shared Traefik container and the
shared DNS container. The one-time OS setup (DNS resolver, trusted CA) is kept.`,
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

		if err := proxy.StopDNSContainer(ctx); err != nil {
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
		return false, errors.New("teardown affects every registered project, run it with --force in non-interactive environments")
	}

	var confirmed bool
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Proceed with the teardown?").
			Value(&confirmed),
	)).WithTheme(tui.ShopwareTheme())
	if err := form.Run(); err != nil {
		return false, err
	}

	if !confirmed {
		fmt.Println(tui.DimText.Render("  Teardown cancelled"))
	}

	return confirmed, nil
}

func init() {
	projectProxyCmd.AddCommand(projectProxyTeardownCmd)
	projectProxyTeardownCmd.Flags().Bool("force", false, "Tear down without asking for confirmation")
}
