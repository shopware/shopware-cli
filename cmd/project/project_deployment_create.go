//go:build deployment

package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/cliversion"
	"github.com/shopware/shopware-cli/internal/deployment"
)

var projectDeploymentCreateCmd = &cobra.Command{
	Use:   "create [project-directory]",
	Short: "Create an immutable deployment",
	Long: `Create an immutable deployment using the selected environment's backend.
For SSH, build a temporary local copy with the archive packaging pipeline and
retain the tar.gz under a memorable name such as focused-wise-turing in
.shopware-cli/deployments. Creation itself does not open an SSH connection.
Report that name as the deployment reference; a custom --output instead reports
its archive path.
Existing archives are never overwritten. With no directory, find the closest Shopware project.
By default, this does not upload or roll out the deployment.
Use --rollout to immediately activate the deployment just created in the selected
environment, with confirmation in interactive sessions. If rollout fails or is
canceled, the created archive is retained for retrying.`,
	Args: cobra.MaximumNArgs(1),
	ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := resolveDeploymentProjectRoot(args)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		withDev, _ := cmd.Flags().GetBool("with-dev-dependencies")
		backend, err := resolveProjectDeploymentBackend(cmd, projectRoot)
		if err != nil {
			return err
		}

		return runProjectDeploymentCreate(cmd, backend, projectRoot, deployment.CreateOptions{
			OutputPath: output, WithDevDependencies: withDev, ToolVersion: cliversion.Version,
		})
	},
}

func runProjectDeploymentCreate(cmd *cobra.Command, backend deployment.Backend, projectRoot string, options deployment.CreateOptions) error {
	artifact, err := backend.CreateDeployment(cmd.Context(), options)
	if err != nil {
		return fmt.Errorf("create deployment: %w", err)
	}
	if artifact.Reference == "" {
		return errors.New("create deployment: backend returned an empty deployment reference")
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Created deployment %q\n", artifact.Reference); err != nil {
		return err
	}
	if rollout, _ := cmd.Flags().GetBool("rollout"); rollout {
		return runProjectDeploymentRollout(cmd, backend, artifact)
	}
	next, err := nextDeploymentRolloutCommand(cmd, projectRoot, artifact.Reference)
	if err != nil {
		return fmt.Errorf("format rollout command: %w", err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "\nDeploy it with:\n  %s\n", next)
	return err
}

// Format a POSIX-shell command without interpreting the backend's reference.
func nextDeploymentRolloutCommand(cmd *cobra.Command, projectRoot, reference string) (string, error) {
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	args := []string{"shopware-cli", "project", "deploy", "rollout"}
	if env, _ := cmd.Flags().GetString("env"); env != "" {
		args = append(args, "-e", quote(env))
	}
	if cmd.Flags().Changed("project-config") {
		config, err := cmd.Flags().GetString("project-config")
		if err != nil {
			return "", err
		}
		config, err = filepath.Abs(config)
		if err != nil {
			return "", err
		}
		args = append(args, "--project-config", quote(config))
	}
	if strings.HasPrefix(reference, "-") {
		args = append(args, "--")
	}
	args = append(args, quote(reference))
	command := strings.Join(args, " ")
	if projectRoot != "" {
		root, err := filepath.Abs(projectRoot)
		if err != nil {
			return "", err
		}
		rootInfo, err := os.Stat(root)
		if err != nil {
			return "", err
		}
		cwdInfo, err := os.Stat(".")
		if err != nil {
			return "", err
		}
		if !os.SameFile(rootInfo, cwdInfo) {
			command = "cd " + quote(root) + " && " + command
		}
	}
	return command, nil
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentCreateCmd)
	projectDeploymentCreateCmd.Flags().StringP("output", "o", "", "Local archive path (default: .shopware-cli/deployments/<generated-name>.tar.gz)")
	projectDeploymentCreateCmd.Flags().Bool("with-dev-dependencies", false, "Install dev dependencies")
	projectDeploymentCreateCmd.Flags().Bool("rollout", false, "Roll out the created deployment immediately")
}
