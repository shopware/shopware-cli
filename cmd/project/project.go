package project

import (
	"github.com/spf13/cobra"
)

var (
	projectConfigPath string
	environmentName   string
)

const environmentFlagUsage = "Environment to target (defaults to environments.local; deprecated top-level url/admin_api is used when environments.local is absent)"

var projectRootCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage your Shopware Project",
}

func Register(rootCmd *cobra.Command) {
	rootCmd.AddCommand(projectRootCmd)
	projectRootCmd.PersistentFlags().StringVar(&projectConfigPath, "project-config", "", "Path to config, if empty searches default location .config/shopware-project.yml first with legacy fallbacks for .shopware-project.yaml and .shopware-project.yml")
	projectRootCmd.PersistentFlags().StringVarP(&environmentName, "env", "e", "", environmentFlagUsage)
}
