package extension

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/system"
)

func newCreateCmd() *cobra.Command {
	opts := &extension.CreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new extension",
		Long:  `Create a new plugin or theme with scaffolding inside a Shopware project.`,
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateInput(opts)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			needsName := opts.Name == ""
			needsStore := !cmd.Flags().Changed("store")

			if needsName {
				if !system.IsInteractionEnabled(cmd.Context()) {
					return errors.New("extension name is required when interaction is disabled")
				}

				if err := runInteractiveCreateForm(opts, needsName, needsStore); err != nil {
					return fmt.Errorf("running create form: %w", err)
				}
			}

			if err := extension.ValidateName(opts.Name, opts.Store); err != nil {
				return err
			}

			return extension.Create(cmd.Context(), *opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.Name, "name", "", "Extension name (PascalCase)")
	flags.BoolVar(&opts.Store, "store", false, "Planning commercial use in Shopware Community Store")
	flags.StringVarP((*string)(&opts.Type), "type", "t", string(extension.Plugin), "Extension type (plugin|theme)")

	_ = flags.MarkHidden("type") // Since "theme" is not implemented yet, this flag is hidden from the user

	_ = cmd.RegisterFlagCompletionFunc("type", cobra.FixedCompletions(
		[]string{string(extension.Plugin), string(extension.Theme)},
		cobra.ShellCompDirectiveNoFileComp,
	))

	return cmd
}

func validateInput(opts *extension.CreateOptions) error {
	if err := extension.ValidateType(opts.Type); err != nil {
		return err
	}
	if opts.Name == "" {
		return nil
	}

	return extension.ValidateName(opts.Name, opts.Store)
}

func init() {
	extensionRootCmd.AddCommand(newCreateCmd())
}
