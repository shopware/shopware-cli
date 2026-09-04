package extension

import (
	"charm.land/huh/v2"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/tui"
)

func printQuestions(opts *extension.CreateOptions) error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[bool]().
				Title("Do you intend to publish this on the Community Store?").
				Description("Hint: In order to publish your extension on the Store a name with a vendor prefix is required.").
				Options(
					huh.NewOption("No, keep it in this project", false),
					huh.NewOption("Yes, Community Store", true),
				).
				Value(&opts.Store),
		),
		huh.NewGroup(
			huh.NewSelect[extension.ExtensionType]().
				Title("Extension type").
				Options(
					huh.NewOption("Plugin", extension.Plugin),
					huh.NewOption("Theme", extension.Theme),
				).
				Value(&opts.Type),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Extension name").
				Description("PascalCase, for example SwagBasicExample.").
				Placeholder("SwagBasicExample").
				Value(&opts.Name).
				Validate(extension.ValidateName),
		),
	)

	return form.Run()
}

func runInteractiveCreateForm(opts *extension.CreateOptions, needsStore bool, needsName bool) error {
	// Print the shopware banner
	tui.PrintBanner()

	// Create the form dynamically, based on needed Input 
	var groups []*huh.Group

	if needsStore {
		groups = append(groups,
			huh.NewGroup(
				huh.NewSelect[bool]().
					Title("Do you intend to publish this on the Community Store?").
					Description("Hint: In order to publish your extension on the Store a name with a vendor prefix is required.").
					Options(
						huh.NewOption("No, keep it in this project", false),
						huh.NewOption("Yes, Community Store", true),
					).
					Value(&opts.Store),
				),
			)
	}

	if needsName {
		groups = append(groups,
			huh.NewGroup(
				huh.NewInput().
					Title("Extension name").
					Description("PascalCase, for example SwagBasicExample.").
					Placeholder("SwagBasicExample").
					Value(&opts.Name).
					Validate(extension.ValidateName),
			),
		)
	}

	if len(groups) == 0 {
		return nil
	}

	return huh.NewForm(groups...).Run()
}


