package extension

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/system"
)

// To ensure consistent naming the flag names are provided as constants
const NameFlagName = "name"
const TypeFlagName = "type"
const VendorFlagName = "vendor" // the user can choose to provide a vendor even if he did not enable --store
const StoreFlagName = "store"

func newCreateCmd() *cobra.Command {
	opts := &extension.CreateOptions{}

	isProvided := make(map[string]bool)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new extension",
		Long:  `Create a new plugin or theme with scaffolding inside a Shopware project.`,
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Collect provided flags
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				isProvided[f.Name] = cmd.Flags().Changed(f.Name)
			})

			interactive := system.IsInteractionEnabled(cmd.Context())

			var errs error

			// Validate the relationships of provided flags
			err := validateFlagRelations(isProvided, opts.Store, interactive)
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("\n%w", err))
			}

			// Validate the values of provided flags
			err = validateFlagValues(opts, isProvided)
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("\n%w", err))
			}

			if errs != nil {
				return errs
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			shouldRunForm := missing(isProvided, opts.Store) && system.IsInteractionEnabled(cmd.Context())

			if shouldRunForm {
				if err := runInteractiveCreateFormWithValidation(opts, isProvided); err != nil {
					return fmt.Errorf("running create form: %w", err)
				}
			}

			return extension.Create(cmd.Context(), *opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.Name, NameFlagName, "", "Extension name (PascalCase)")
	flags.StringVar(&opts.Vendor, VendorFlagName, "", "Vendor prefix (PascalCase) for the extension name and namespace. Required if --store is enabled.")
	flags.BoolVar(&opts.Store, StoreFlagName, false, "Enable if you plan to publish the extension on the Shopware Community Store.")
	flags.StringVarP((*string)(&opts.Type), TypeFlagName, "t", "", "Extension type (plugin|theme)")

	_ = cmd.RegisterFlagCompletionFunc("type", cobra.FixedCompletions(
		[]string{string(extension.Plugin), string(extension.Theme)},
		cobra.ShellCompDirectiveNoFileComp,
	))

	return cmd
}

func init() {
	extensionRootCmd.AddCommand(newCreateCmd())
}

func missing(isProvided map[string]bool, store bool) bool {
	required := []string{NameFlagName, TypeFlagName}

	if isProvided[StoreFlagName] && store {
		required = append(required, VendorFlagName)
	}

	for _, flagName := range required {
		if !isProvided[flagName] {
			return true
		}
	}

	return false
}

func validateFlagRelations(isProvided map[string]bool, store bool, interactive bool) error {
	if !interactive {
		var errs error
		requiredFlags := []string{NameFlagName, TypeFlagName}
		for _, flagName := range requiredFlags {
			if !isProvided[flagName] {
				errs = errors.Join(errs, fmt.Errorf("required flag missing: --%s is required in non-interactive mode", flagName))
			}
		}

		if store && !isProvided[VendorFlagName] {
			errs = errors.Join(errs, errors.New("required flag missing: --vendor is required when --store is enabled"))
		}

		if errs != nil {
			return errs
		}
	}
	return nil
}

func validateFlagValues(opts *extension.CreateOptions, isProvided map[string]bool) error {
	var errs []error

	if isProvided[VendorFlagName] {
		if err := extension.ValidateVendor(opts.Vendor); err != nil {
			errs = append(errs, err)
		}
	}

	if isProvided[NameFlagName] {
		if err := extension.ValidateName(opts.Name); err != nil {
			errs = append(errs, err)
		}
	}

	if isProvided[TypeFlagName] {
		if err := extension.ValidateType(opts.Type); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
