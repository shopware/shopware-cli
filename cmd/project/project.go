package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
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
	projectRootCmd.PersistentFlags().StringVar(&projectConfigPath, "project-config", shop.DefaultConfigFileName(), "Path to config")
	projectRootCmd.PersistentFlags().StringVarP(&environmentName, "env", "e", "", environmentFlagUsage)
}
