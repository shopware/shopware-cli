package project

import (
	"github.com/spf13/cobra"
)

var (
	projectConfigPath string
	environmentName   string
)

const environmentFlagUsage = "Select the environment to target (defaults to environments.local; falls back to deprecated top-level url/admin_api if absent)"

var projectRootCmd = &cobra.Command{
	Use:   "project",
	Short: "Create, develop, build, validate, and deploy Shopware projects",
}

func Register(rootCmd *cobra.Command) {
	rootCmd.AddCommand(projectRootCmd)
	projectRootCmd.PersistentFlags().StringVar(&projectConfigPath, "project-config", "", "Path to config, if empty searches default location .config/shopware-project.yml first with legacy fallbacks for .shopware-project.yaml and .shopware-project.yml")
	projectRootCmd.PersistentFlags().StringVarP(&environmentName, "env", "e", "", environmentFlagUsage)
	_ = projectRootCmd.RegisterFlagCompletionFunc("env", completeEnvironmentNames)
}
