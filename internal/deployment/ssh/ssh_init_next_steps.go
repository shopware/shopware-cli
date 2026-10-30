package ssh

import (
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/internal/deployment"
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
		args = append(args, "--project-config", quote(configPath))
	}
	args = append(args, "--", quote(root))

	var summary strings.Builder
	summary.WriteString("\nRuntime configuration (add custom environment variables here):\n")
	for _, target := range targets {
		deploymentRoot, err := target.deploymentRoot()
		if err != nil {
			return err
		}
		fmt.Fprintf(&summary, "  %q: %q\n", target.env.SSH.Host, path.Join(deploymentRoot, "shared", ".env.local"))
	}
	summary.WriteString("These files are shared by deployment releases.\n")
	summary.WriteString("Initialization runs once. Edit these files directly for later configuration changes.\n")
	summary.WriteString("\nNext steps:\n")
	summary.WriteString("  1. Ensure the configured database and database user exist and are reachable from the SSH hosts.\n")
	summary.WriteString("  2. Configure each web server's document root:\n")
	for _, target := range targets {
		fmt.Fprintf(&summary, "     %q: %q\n", target.env.SSH.Host, path.Join(target.directory, "public"))
	}
	summary.WriteString("     For a first deployment, this path becomes available during rollout.\n")
	summary.WriteString("  3. Build a deployment archive from your local machine:\n     ")
	summary.WriteString(strings.Join(args, " "))
	summary.WriteString("\n     Archive creation does not upload or activate a release.\n  4. Configure background workers and scheduled tasks to run from the active release path (ssh.directory).\n")
	summary.WriteString("  5. After rollout, open your application URL and verify the storefront and Administration.\n")
	if _, err := io.WriteString(output, summary.String()); err != nil {
		return fmt.Errorf("cannot display deployment next steps: %w", err)
	}
	return nil
}
