package extension

import (
	"charm.land/huh/v2"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/tui"
)

func runInteractiveCreateForm(opts *extension.CreateOptions, needsName bool, needsStore bool) error {
	// Print the shopware banner
	tui.PrintBanner()

	// Create the form dynamically based on required input.
	var groups []*huh.Group

	if needsStore {
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

	if needsName {
		groups = append(groups,
			huh.NewGroup(
				huh.NewInput().
					Title("Extension Name").
					Description("Use PascalCase and, for Community Store extensions, a vendor prefix, e.g. SwagBasicExample.").
					Placeholder("SwagBasicExample").
					Value(&opts.Name).
					Validate(func(name string) error {
						return extension.ValidateName(name, opts.Store)
					}),
			),
		)
	}

	if len(groups) == 0 {
		return nil
	}

	return huh.NewForm(groups...).Run()
}
