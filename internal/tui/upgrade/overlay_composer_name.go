package upgrade

import (
	backend "github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui/textprompt"
)

// composerNamePromptKey marks results coming from the package-name prompt.
type composerNamePromptKey struct{}

// newComposerNamePrompt builds the overlay asking for the project's Composer
// package name. value pre-fills the input (the suggested name on first open,
// the rejected input after a validation error); prevErr carries that
// validation error back into the prompt.
func newComposerNamePrompt(projectRoot, value string, prevErr error) *textprompt.Overlay {
	if value == "" {
		value = backend.SuggestComposerName(projectRoot)
	}

	help := "Composer requires a package name in composer.json and refuses to run without one.\n" +
		"The name is written into composer.json; nothing else changes."
	if prevErr != nil {
		help = prevErr.Error() + "\n\n" + help
	}

	return textprompt.New(textprompt.Options{
		Key:         composerNamePromptKey{},
		Title:       "Set a Composer package name",
		Help:        help,
		Value:       value,
		Placeholder: "vendor/package, e.g. shopware/production",
	})
}
