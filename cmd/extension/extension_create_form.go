package extension

import (
	"charm.land/huh/v2"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/tui"
)

func runInteractiveCreateFormWithValidation(opts *extension.CreateOptions, isProvided map[string]bool) error {
	// Print the shopware banner
	tui.PrintBanner()

	// Define the theme for the interactive form.
	theme := huh.ThemeFunc(func(isDark bool) *huh.Styles {
		s := huh.ThemeCharm(isDark)
		s.Focused.Title = s.Focused.Title.Foreground(tui.BlueColor)
		s.Blurred.Title = s.Blurred.Title.Foreground(tui.BlueColor)
		return s
	})

	// Create the form dynamically based on required input.
	var groups []*huh.Group

	if !isProvided[TypeFlagName] {
		groups = append(groups,
			huh.NewGroup(
				huh.NewSelect[extension.ExtensionType]().
					Title("Extension Type").
					Description("Choose the type of extension you want to create.").
					Options(
						huh.NewOption("Plugin", extension.Plugin),
						huh.NewOption("Theme", extension.Theme),
					).
					Value(&opts.Type),
			),
		)
	}

	if !isProvided[StoreFlagName] {
		groups = append(groups,
			huh.NewGroup(
				huh.NewSelect[bool]().
					Title("Do you plan to publish this extension in the Community Store?").
					Description("This affects where the extension is created. Store extensions require a vendor-prefixed name.").
					Options(
						huh.NewOption("No, it's only for this project.", false),
						huh.NewOption("Yes, I plan to publish it.", true),
					).
					Value(&opts.Store),
			),
		)
	}

	if !isProvided[VendorFlagName] {
		groups = append(groups,
			huh.NewGroup(
				huh.NewInput().
					Title("Vendor Prefix").
					Description("Provide a vendor prefix in PascalCase.").
					Placeholder("MyVendor").
					Value(&opts.Vendor).
					Validate(extension.ValidateVendor),
			).WithHideFunc(func() bool {
				return !opts.Store
			}),
		)
	}

	if !isProvided[NameFlagName] {
		groups = append(groups,
			huh.NewGroup(
				huh.NewInput().
					Title("Extension Name").
					Description("Provide a name in PascalCase, e.g. MyExtension.").
					Placeholder("MyExtension").
					Value(&opts.Name).
					Validate(extension.ValidateName),
			),
		)
	}

	if len(groups) == 0 {
		return nil
	}

	return huh.NewForm(groups...).WithTheme(theme).Run()
}
