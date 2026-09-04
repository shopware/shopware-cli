package project

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectProxyStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Report whether the current project is registered with the shared proxy",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := findClosestShopwareProject(false)
		if err != nil {
			return err
		}

		return proxyStatus(cmd, projectRoot)
	},
}

var projectProxyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every project registered with the shared proxy",
	RunE: func(cmd *cobra.Command, args []string) error {
		return proxyList(cmd)
	},
}

func proxyStatus(cmd *cobra.Command, projectRoot string) error {
	ctx := cmd.Context()
	canonicalRoot := proxy.CanonicalProjectRoot(projectRoot)

	reg, err := proxy.LoadRegistry()
	if err != nil {
		return err
	}

	entry, found := reg.Find(canonicalRoot)
	if !found {
		fmt.Println(tui.RedText.Bold(true).Render("  ✗ Not registered with the shared proxy"))
		fmt.Println(tui.DimText.Render("  Run ") + tui.BoldText.Render("shopware-cli project proxy up") + tui.DimText.Render(" to register it."))
		return ErrProxyNotRegistered
	}

	fmt.Println(tui.GreenText.Bold(true).Render("  ✓ Registered with the shared proxy"))
	fmt.Println(tui.DimText.Render("  Hostname: ") + tui.BoldText.Render(entry.Hostname))

	if proxy.ContainerIsRunning(ctx) {
		fmt.Println(tui.DimText.Render("  Shared proxy: ") + tui.GreenText.Render("running"))
	} else {
		fmt.Println(tui.DimText.Render("  Shared proxy: ") + tui.RedText.Render("not running"))
	}

	return nil
}

func proxyList(cmd *cobra.Command) error {
	ctx := cmd.Context()

	reg, err := proxy.LoadRegistry()
	if err != nil {
		return err
	}

	if len(reg.Projects) == 0 {
		fmt.Println(tui.DimText.Render("  No projects are registered. Run \"shopware-cli project proxy up\" inside a shop."))
		return nil
	}

	instances, err := proxy.RunningInstances(ctx)
	if err != nil {
		return err
	}

	for _, entry := range reg.Projects {
		running := projectIsRunning(entry, instances)

		status := tui.RedText.Render("stopped")
		if running {
			status = tui.GreenText.Render("running")
		}

		fmt.Println()
		fmt.Printf("  %s  %s  %s\n", tui.SectionTitleStyle.Render(entry.Hostname), status, tui.DimText.Render(entry.ProjectRoot))

		if !running {
			continue
		}

		for _, link := range projectLinks(entry, instances) {
			fmt.Printf("    %s %s\n", tui.DimText.Render(fmt.Sprintf("%-9s", link.Label)), link.URL)
		}
	}
	fmt.Println()

	return nil
}

// projectLinks builds the links for a running project: the shop and admin on
// the project hostname, plus one subdomain link per running service with a
// web UI.
func projectLinks(entry proxy.ProjectEntry, instances []proxy.Instance) []docker.Link {
	links := []docker.Link{
		{Label: "Shop", URL: "https://" + entry.Hostname},
		{Label: "Admin", URL: "https://" + entry.Hostname + "/admin"},
	}

	for _, service := range runningServices(entry, instances) {
		if link, ok := docker.ServiceLink(service, entry.Hostname); ok {
			links = append(links, link)
		}
	}

	return links
}

// runningServices extracts the compose service names of entry's running
// containers, which are named <project>-<service>-<index>.
func runningServices(entry proxy.ProjectEntry, instances []proxy.Instance) []string {
	prefix := proxy.ComposeProjectName(entry.ProjectRoot) + "-"

	var services []string
	for _, inst := range instances {
		name, found := strings.CutPrefix(inst.Container, prefix)
		if !found {
			continue
		}

		if idx := strings.LastIndex(name, "-"); idx > 0 {
			services = append(services, name[:idx])
		}
	}

	slices.Sort(services)

	return services
}

// projectIsRunning reports whether any running container belongs to entry's
// project, matched by its Docker Compose project name (the unique
// COMPOSE_PROJECT_NAME `project create` writes, or Compose's sanitized default).
func projectIsRunning(entry proxy.ProjectEntry, instances []proxy.Instance) bool {
	prefix := proxy.ComposeProjectName(entry.ProjectRoot) + "-"
	for _, inst := range instances {
		if strings.HasPrefix(inst.Container, prefix) {
			return true
		}
	}

	return false
}

func init() {
	projectProxyCmd.AddCommand(projectProxyStatusCmd)
	projectProxyCmd.AddCommand(projectProxyListCmd)
}
