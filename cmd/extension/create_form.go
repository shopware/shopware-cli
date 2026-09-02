package extension

import (
	"charm.land/huh/v2"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/tui"
)

func runCreateForm(opts *extension.CreateOptions) error {
	usageOptions := []huh.Option[extension.ExtensionUsage]{
		huh.NewOption("Private project", extension.PrivateUsage),
		huh.NewOption("Shopware Community Store", extension.CommercialUsage),
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[extension.ExtensionUsage]().
			Title("How will the extension be used?").
			Description("Private extensions live in custom/static-plugins. Store extensions live in custom/plugins and require a vendor prefix.").
			Options(usageOptions...).
			Value(&opts.Usage),
		huh.NewInput().
			Title("Extension name").
			Description("Use UpperCamelCase with a vendor prefix, for example SwagBasicExample.").
			Placeholder("SwagBasicExample").
			Value(&opts.Name).
			Validate(extension.IsValidName),
	))

	return form.WithTheme(tui.ShopwareTheme()).Run()
}
