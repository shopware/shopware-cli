package extension

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/extension/scaffolding"
)

func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an example implementation to a plugin",
		Long: `Add an example implementation of a single Shopware feature to the plugin in the
current directory. Generators only create missing files and extend the service
and route configuration, existing code is never changed.`,
	}

	for _, generator := range extension.Generators() {
		cmd.AddCommand(newGeneratorCmd(generator))
	}

	return cmd
}

func newGeneratorCmd(generator scaffolding.Generator) *cobra.Command {
	use := generator.Name
	args := cobra.NoArgs

	if generator.Args != "" {
		use += " " + generator.Args
		args = cobra.MinimumNArgs(1)
	}

	return &cobra.Command{
		Use:   use,
		Short: generator.Short,
		Args:  args,
		RunE: func(cmd *cobra.Command, args []string) error {
			return extension.Add(cmd.Context(), generator, args)
		},
	}
}

func init() {
	extensionRootCmd.AddCommand(newAddCmd())
}
