package extension

import (
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	// CreateOptions holds the options for creating a new extension.
	opts := &extension.CreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new extension",
		Long:  "Create a new extension with the specified parameters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return extension.Create(cmd.Context(), *opts)
		},
	}

	// Define flags for the command
	fs := cmd.Flags()
	fs.StringVar(&opts.Name, "name", "", "Name of the extension")
	fs.StringVar((*string)(&opts.Usage), "usage", string(extension.ExtensionUsagePrivate), "Usage of the extension (private or store)")
	fs.StringVar((*string)(&opts.Type), "type", string(extension.ExtensionTypePlugin), "Type of the extension (plugin or theme)")

	return cmd
}

func init() {
	extensionRootCmd.AddCommand(newCreateCmd())
}
