package project

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectProxyStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Report whether the current project is registered with the shared proxy",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := findClosestShopwareProject()
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
			fmt.Printf("    %s %s\n", tui.DimText.Render(fmt.Sprintf("%-9s", link.label)), link.url)
		}
	}
	fmt.Println()

	return nil
}

// serviceLink is one URL shown below a project in `proxy list`.
type serviceLink struct {
	label string
	url   string
}

// proxiedServiceLabels maps compose service names to the label shown for
// their subdomain link. Only services with a web UI are listed.
var proxiedServiceLabels = map[string]string{
	"adminer":    "Adminer",
	"mailer":     "Mailpit",
	"lavinmq":    "Queue",
	"opensearch": "Search",
}

// projectLinks builds the links for a running project: the shop and admin on
// the project hostname, plus one subdomain link per running service with a
// web UI.
func projectLinks(entry proxy.ProjectEntry, instances []proxy.Instance) []serviceLink {
	links := []serviceLink{
		{label: "Shop", url: "https://" + entry.Hostname},
		{label: "Admin", url: "https://" + entry.Hostname + "/admin"},
	}

	for _, service := range runningServices(entry, instances) {
		if label, ok := proxiedServiceLabels[service]; ok {
			links = append(links, serviceLink{label: label, url: fmt.Sprintf("https://%s.%s", service, entry.Hostname)})
		}
	}

	return links
}

// runningServices extracts the compose service names of entry's running
// containers, which are named <project>-<service>-<index>.
func runningServices(entry proxy.ProjectEntry, instances []proxy.Instance) []string {
	prefix := filepath.Base(entry.ProjectRoot) + "-"

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
// project, matched by the compose project name Docker Compose derives from
// the project directory's basename.
func projectIsRunning(entry proxy.ProjectEntry, instances []proxy.Instance) bool {
	prefix := filepath.Base(entry.ProjectRoot) + "-"
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
