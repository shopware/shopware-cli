package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
)

var projectConfigSchemaCmd = &cobra.Command{
	Use:   "config-schema",
	Short: "Print the JSON schema for the .shopware-project.yml configuration",
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := cmd.OutOrStdout().Write(shop.ConfigSchema())
		return err
	},
}

func init() {
	projectRootCmd.AddCommand(projectConfigSchemaCmd)
}
