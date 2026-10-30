//go:build deployment

package project

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
	deploymentbackend "github.com/shopware/shopware-cli/internal/deployment/backend"
	"github.com/shopware/shopware-cli/internal/system"
)

var projectDeploymentInitCmd = &cobra.Command{
	Use:   "init [project-directory]",
	Short: "Initialize a deployment environment",
	Example: `  shopware-cli project deploy init -e production
  shopware-cli -n project deploy init -e production --init-config - < init.json`,
	Args: cobra.MaximumNArgs(1),
	ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	},
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		configPath, _ := cmd.Flags().GetString("init-config")
		if configPath == "" && !system.IsInteractionEnabled(cmd.Context()) {
			return errors.New("cannot initialize deployment without interaction: pass --init-config with a JSON file or '-' for stdin")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolveDeploymentProjectRoot(args)
		if err != nil {
			return err
		}
		target, err := deploymentbackend.ResolveBackend(cmd.Context(), root, projectConfigPath, environmentName)
		if err != nil {
			return err
		}
		return runProjectDeploymentInit(cmd, target)
	},
}

// runProjectDeploymentInit initializes a supported backend and prints its setup guidance after success.
func runProjectDeploymentInit(cmd *cobra.Command, target deployment.Backend) error {
	initializer, ok := target.(deployment.Initializer)
	if !ok {
		return fmt.Errorf("cannot initialize deployments with backend %q: %w", target.Type(), deployment.ErrNotSupported)
	}
	options := deployment.InitializationOptions{}
	configPath, _ := cmd.Flags().GetString("init-config")
	if configPath != "" {
		input := cmd.InOrStdin()
		if configPath != "-" {
			file, err := os.Open(configPath)
			if err != nil {
				return fmt.Errorf("cannot open initialization config: %w", err)
			}
			defer func() { _ = file.Close() }()
			input = file
		}
		config, err := deployment.ReadInitializationConfig(input)
		if err != nil {
			return err
		}
		options.Config = config
	}
	if err := initializer.InitializeDeployment(cmd.Context(), options); err != nil {
		if errors.Is(err, deployment.ErrInitializationCanceled) {
			return nil
		}
		return fmt.Errorf("cannot initialize deployment: %w", err)
	}
	if guide, ok := target.(deployment.InitializationGuide); ok {
		environment, _ := cmd.Flags().GetString("env")
		return guide.WriteInitializationNextSteps(environment, cmd.OutOrStdout())
	}
	return nil
}

func init() {
	projectDeploymentInitCmd.Flags().String("init-config", "", "Read initialization values from a JSON file ('-' for stdin), without prompts")
	projectDeploymentCmd.AddCommand(projectDeploymentInitCmd)
}
