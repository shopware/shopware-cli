package extension

import (
	"charm.land/huh/v2"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/tui"
)

func printQuestions(opts *extension.CreateOptions) error {
	tui.PrintBanner()
	
	usageOptions := []huh.Option[extension.ExtensionUsage]{
		huh.NewOption("No, keep it in this project", extension.PrivateUsage),
		huh.NewOption("Yes, Community Store", extension.CommercialUsage),
	}

	form := huh.NewForm(
      	huh.NewGroup(
          	huh.NewSelect[extension.ExtensionUsage]().
				Title("Do you intend to publish this on the Community Store?").
				Description("Hint: In order to publish your extension on the Store a name with a vendor prefix is required.").
				Options(usageOptions...).
				Value(&opts.Usage),
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

	return form.WithTheme(tui.ShopwareTheme()).Run()
}

func printSummary(name string) error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Summary").
				Description("Extension name: " + name),
		),
	)

	return form.WithTheme(tui.ShopwareTheme()).Run()
}

func runInteractiveCreateForm(opts *extension.CreateOptions) error {
	err := printQuestions(opts)
	if err != nil {
		return err
	}

	err = printSummary(opts.Name)
		if err != nil {
		return err
	}
	return nil
}