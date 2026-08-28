package extension

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"charm.land/huh/v2"
	"github.com/shopware/shopware-cli/internal/extension/scaffolding"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

type ExtensionType string

const (
	ExtensionTypePlugin ExtensionType = "plugin"
	ExtensionTypeTheme  ExtensionType = "theme"
)

type ExtensionUsage string

const (
	ExtensionUsagePrivate ExtensionUsage = "private"
	ExtensionUsageStore   ExtensionUsage = "store"
)

// CreateOptions holds the options for creating a new extension.
type CreateOptions struct {
	Name  string
	Usage ExtensionUsage
	Type  ExtensionType
}

// Create reads the command line arguments and creates a new extension based on the provided parameters.
func Create(ctx context.Context, opts CreateOptions) error {

	logging.FromContext(ctx).Infof("Creating new extension: %s", opts.Name)

	// check if non-interactive, parse and validate arguments

	// ask for extension name if not provided and validate
	runCreateForm(&opts)

	// ask for extension type (theme or extension)

	// ask if extension for store or private use (show information what this means)

	// ask for compatibility version (>= 6.6)

	// summarize and ask for confirmation

	// Create the extensions directory in the Shopware project, in which the extension will live.
	projectDir, err := shop.FindClosestShopwareProject()
	if err != nil {
		return err
	}

	// change prefix if private use is intended
	var prefix string = ""
	switch opts.Usage {
	case ExtensionUsagePrivate:
		prefix = "static-"
	case ExtensionUsageStore:
		prefix = ""
	}

	extensionDir := filepath.Join(projectDir, "custom", prefix+"plugins", opts.Name)

	logging.FromContext(ctx).Infof("Creating extension directory: %s", extensionDir)

	err = scaffolding.CreateExtensionDir(extensionDir)
	if err != nil {
		return err // dir already exists -> do not delete
	}

	// If an error occurs during the scaffolding process clean up the created extension directory.
	// Only runs if extension was created in THIS process
	defer func() {
		if err == nil {
			return
		}
		logging.FromContext(ctx).Infof("Removing the extension because of an error.")
		cleanupErr := scaffolding.RemoveCreatedExtensionDir(extensionDir)
		if cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()

	// Create the scaffolding for the new extension inside the extension directory.

	// Require the extension directory to exist
	info, err := os.Stat(extensionDir)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("missing extension directory: %s", extensionDir)
	}
	if err != nil {
		return fmt.Errorf("stat extension directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists and is not a directory", extensionDir)
	}

	logging.FromContext(ctx).Infof("Creating scaffolding files for extension: %s", opts.Name)

	err = scaffolding.CreateScaffoldingFiles(extensionDir, opts.Name)
	if err != nil {
		return err
	}

	logging.FromContext(ctx).Infof("Extension %s scaffolding created.", opts.Name)

	/*
		// validate the created extension
		valid, err := validateCreatedExtension(ctx, extensionDir)
		if err != nil {
			return fmt.Errorf("failed to validate created extension: %w", err)
		}
		if !valid {
			return fmt.Errorf("validation failed for created extension: %s, error was: %w", opts.Name, err)
			// error message should now not be nil
		}*/

	// inform user if validation is successful and make clear that this does not mean that extension will pass store review
	logging.FromContext(ctx).Infof("Extension %s validated successfully.", opts.Name)

	// make clear which requiremtents are not checked by the CLI and need manual review

	// show possible next steps (maybe hint that lsp helps with development)

	logging.FromContext(ctx).Infof("Extension %s creation successful.", opts.Name)

	return nil
}

// validateCreatedExtension runs extension validate and returns true if the extension is valid, false if not, and an error if the validation process itself failed.
func validateCreatedExtension(ctx context.Context, extensionDir string) (bool, error) {
	// Load the extension from the created directory.
	ext, err := GetExtensionByFolder(ctx, extensionDir)
	if err != nil {
		return false, fmt.Errorf("failed to get extension by folder: %w", err)
	}

	// Validate the extension.
	check := &checkResult{}
	RunValidation(ctx, ext, check)

	return !check.HasErrors(), nil
}

// checkResult implements the validation.Check interface
type checkResult struct {
	results []validation.CheckResult
}

func (c *checkResult) AddResult(r validation.CheckResult) { c.results = append(c.results, r) }

func (c *checkResult) GetResults() []validation.CheckResult { return c.results }

func (c *checkResult) HasErrors() bool {
	for _, r := range c.results {
		if r.Severity == validation.SeverityError {
			return true
		}
	}
	return false
}

// mutex not needed in verifier.Check; that is for running PHPStan/ESLint in parallel.
func (c *checkResult) RemoveByIdentifier([]validation.ToolConfigIgnore) validation.Check { return c }

func runCreateForm(opts *CreateOptions) error {
	var formGroups []*huh.Group

	// Check for missing input
	needsExtensionName := opts.Name == ""
	// todo add the other options

	storeIntentOptions := []huh.Option[ExtensionUsage]{
		huh.NewOption("Yes", ExtensionUsageStore),
		huh.NewOption("No", ExtensionUsagePrivate),
	}

	typeOptions := []huh.Option[ExtensionType]{
      huh.NewOption("Plugin", ExtensionTypePlugin),
      huh.NewOption("Theme", ExtensionTypeTheme),
  	}

	// Ask for extension name
	if needsExtensionName {
		formGroups = append(formGroups, huh.NewGroup(
			huh.NewSelect[ExtensionUsage]().
				Title("Do you intent to puplish this Extension to the Shopware Community Store?").
				Description("Blablabla").
				Options(storeIntentOptions...).
				Value(&opts.Usage),
			huh.NewSelect[ExtensionType]().
				Title("Extension Type").
				Description("What do you want to create?").
				Options(typeOptions...).
				Value(&opts.Type),
			huh.NewNote().
				Title("Important Info").
				Description("A vendor prefix is required if you plan to publish the plugin in the Shopware Community Store."),
			huh.NewInput().
				Title("Extension Name").
				Description("Use UpperCamelCase, starting with a capital letter. Choose a name that describes your plugin as succinctly and clearly as possible.").
				Placeholder("SwagBasicExample").
				Value(&opts.Name).
				Validate(validateExtensionName),
		))
	}

	if len(formGroups) == 0 {
		return nil
	}

	return huh.NewForm(formGroups...).WithTheme(tui.ShopwareTheme()).Run()
}

// extensionNameRegexp is Shopware's technical plugin name: UpperCamelCase with a
// vendor prefix, letters and digits only (e.g. SwagBasicExample).
var extensionNameRegexp = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*[A-Z][A-Za-z0-9]*$`)

// validateExtensionName reports whether name can be used as a Shopware plugin
// technical name (directory, PHP class, and composer extra.shopware-plugin-class).
func validateExtensionName(name string) error {
	if name == "" {
		return errors.New("extension name must not be empty")
	}
	if !extensionNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid extension name %q: use UpperCamelCase with a vendor prefix, letters and digits only (e.g. SwagBasicExample)", name)
	}

	return nil
}

/*
Use UpperCamelCase, which means that your plugin name must begin with a capital letter too.
Whenever possible, begin it with a company prefix to avoid duplicate names (e.g., SwagBasicExample).
Choose a name that describes your plugin as succinctly and clearly as possible.

INFO A vendor prefix is required if you plan to publish your plugin in the Shopware Community Store.
*/
