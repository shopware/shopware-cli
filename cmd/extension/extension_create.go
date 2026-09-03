package extension

import (
	"errors"
	"fmt"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	opts := &extension.CreateOptions{}

	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new extension",
		Long:  `Create a new extension with scaffolding inside a Shopware project.`,
		Args:  cobra.MatchAll(cobra.MaximumNArgs(1), validateNameArg),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			switch opts.Usage {
			case extension.PrivateUsage, extension.CommercialUsage:
				return nil
			default:
				return fmt.Errorf("invalid --usage: %s (want private|store)", opts.Usage)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.Name = args[0]
			} else if len(args) == 0 {
				if !system.IsInteractionEnabled(cmd.Context()) {
					return errors.New("extension name is required when interaction is disabled")
				}

				if err := runInteractiveCreateForm(opts); err != nil {
					return fmt.Errorf("running create form: %w", err)
				}
			}

			return extension.Create(cmd.Context(), *opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP((*string)(&opts.Usage), "usage", "u", string(extension.PrivateUsage), "Extension usage (private|store)")

	return cmd
}

// validateNameArg rejects a name that cannot be used as an extension name.
func validateNameArg(_ *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}

	return extension.ValidateName(args[0])
}

func init() {
	extensionRootCmd.AddCommand(newCreateCmd())
}
