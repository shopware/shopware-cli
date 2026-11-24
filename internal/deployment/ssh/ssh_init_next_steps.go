package ssh

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/tui"
)

var _ deployment.InitializationGuide = (*SSH)(nil)
var _ deployment.InitializationGuide = (*sshGroup)(nil)

// WriteInitializationNextSteps prints configuration locations and follow-up commands for the deployment targets.
func (s *SSH) WriteInitializationNextSteps(environment string, output io.Writer) error {
	return writeSSHInitializationNextSteps(s, []*SSH{s}, environment, output)
}

// WriteInitializationNextSteps prints configuration locations and follow-up commands for the deployment targets.
func (g *sshGroup) WriteInitializationNextSteps(environment string, output io.Writer) error {
	targets := make([]*SSH, 0, len(g.hosts))
	for _, host := range g.hosts {
		targets = append(targets, host.backend)
	}
	return writeSSHInitializationNextSteps(g.hosts[g.migration].backend, targets, environment, output)
}

// writeSSHInitializationNextSteps preserves the environment and config selection in shell-quoted deployment instructions.
func writeSSHInitializationNextSteps(source *SSH, targets []*SSH, environment string, output io.Writer) error {
	root, err := filepath.Abs(source.root)
	if err != nil {
		return fmt.Errorf("cannot resolve project directory for next steps: %w", err)
	}
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	args := []string{"shopware-cli", "project", "deploy", "create"}
	if environment != "" {
		args = append(args, "-e", quote(environment))
	}
	if source.configPath != "" {
		configPath, err := filepath.Abs(source.configPath)
		if err != nil {
			return fmt.Errorf("cannot resolve project config for next steps: %w", err)
		}
		if configPath != filepath.Join(root, ".config", "shopware-project.yml") {
			args = append(args, "--project-config", quote(configPath))
		}
	}
	args = append(args, "--rollout")
	cwd, err := os.Getwd()
	if err != nil || cwd != root {
		args = append(args, "--", quote(root))
	}

	heading := func(text string) string { return tui.SectionTitleStyle.Render(text) }
	var summary strings.Builder
	fmt.Fprintln(&summary, "\n"+lipgloss.NewStyle().Foreground(tui.SuccessColor).Bold(true).Render("✓ Deployment environment is ready"))
	fmt.Fprintln(&summary, "\n"+heading("Runtime configuration"))
	for _, target := range targets {
		deploymentRoot, err := target.deploymentRoot()
		if err != nil {
			return err
		}
		fmt.Fprintf(&summary, "  %q: %q\n", target.env.SSH.Host, path.Join(deploymentRoot, "shared", ".env.local"))
	}
	fmt.Fprintln(&summary, tui.DimStyle.Render("  Edit custom environment variables here. This file is shared across releases."))
	fmt.Fprintln(&summary, "\n"+heading("1. Prepare your server"))
	fmt.Fprintln(&summary, "  Make sure PHP 8.4 or higher is installed and the SSH user is the same user that runs PHP.")
	fmt.Fprintln(&summary, "  Make sure the database and database user exist and are reachable.")
	fmt.Fprintln(&summary, "  Point your web server's document root to:")
	for _, target := range targets {
		fmt.Fprintf(&summary, "    %q: %q\n", target.env.SSH.Host, path.Join(target.directory, "public"))
	}
	fmt.Fprintln(&summary, tui.DimStyle.Render("  This directory becomes available during the first rollout."))
	if len(targets) > 1 {
		fmt.Fprintln(&summary, "  This environment deploys to multiple hosts, so they must all see the same files:")
		fmt.Fprintln(&summary, "    Share the \"shared\" directory between all hosts (for example with NFS),")
		fmt.Fprintln(&summary, "    or configure S3 storage for every Shopware filesystem.")
		fmt.Fprintln(&summary, "  With S3 storage, \"shared/.env.local\" is still a local file:")
		fmt.Fprintln(&summary, "    keep it identical on every host and copy each change to all of them manually.")
	}
	fmt.Fprintln(&summary, "\n"+heading("2. Build and roll out your first deployment"))
	fmt.Fprintln(&summary, "  Run locally:")
	fmt.Fprintln(&summary)
	fmt.Fprintln(&summary, "    "+tui.BoldStyle.Render(strings.Join(args, " ")))
	fmt.Fprintln(&summary, "\n  This builds the archive and rolls it out to the selected environment.")
	fmt.Fprintln(&summary, "\n"+heading("3. Finish setup"))
	fmt.Fprintln(&summary, "  Configure background workers and scheduled tasks to use the active release:")
	for _, target := range targets {
		fmt.Fprintf(&summary, "    %q: %q\n", target.env.SSH.Host, target.directory)
	}
	fmt.Fprintln(&summary, "  Open your application URL and check the storefront and Administration.")
	if _, err := lipgloss.Fprint(output, summary.String()); err != nil {
		return fmt.Errorf("cannot display deployment next steps: %w", err)
	}
	return nil
}
