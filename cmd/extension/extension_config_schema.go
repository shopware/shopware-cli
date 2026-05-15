package extension

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
)

var extensionConfigSchemaCmd = &cobra.Command{
	Use:   "config-schema",
	Short: "Print the JSON schema for the .shopware-extension.yml configuration",
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := cmd.OutOrStdout().Write(extension.ConfigSchema())
		return err
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionConfigSchemaCmd)
}
