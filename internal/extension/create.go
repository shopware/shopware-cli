package extension

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

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

	// validate the created extension
	valid, err := validateCreatedExtension(ctx, extensionDir)
	if err != nil {
		return fmt.Errorf("failed to validate created extension: %w", err)
	}
	if !valid {
		return fmt.Errorf("validation failed for created extension: %s, error was: %w", opts.Name, err)
		// error message should now not be nil
	}

	// inform user if validation is successful and make clear that this does not mean that extension will pass store review
	logging.FromContext(ctx).Infof("Extension %s validated successfully.", opts.Name)

	// make clear which requiremtents are not checked by the CLI and need manual review

	// show possible next steps (maybe hint that lsp helps with development)

	logging.FromContext(ctx).Infof("Extension %s creation successful.", opts.Name)

	return nil
}

// validateCreatedExtension checks that the created extension can be installed by
// Shopware and returns true if it is valid, false if not, and an error if the
// validation process itself failed.
func validateCreatedExtension(ctx context.Context, extensionDir string) (bool, error) {
	// Load the extension from the created directory.
	ext, err := GetExtensionByFolder(ctx, extensionDir)
	if err != nil {
		return false, fmt.Errorf("failed to get extension by folder: %w", err)
	}

	// Only a composer.json of the type shopware-platform-plugin is loaded as plugin.
	plugin, ok := ext.(*PlatformPlugin)
	if !ok {
		return false, fmt.Errorf("%s is not a %s", extensionDir, ComposerTypePlugin)
	}

	// Validate the extension.
	check := &checkResult{}
	validatePluginInstallable(plugin, check)

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
				Validate(validateExtensionNameInput),
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

// validateExtensionNameInput reports whether name can be used as a Shopware plugin
// technical name (directory, PHP class, and composer extra.shopware-plugin-class).
func validateExtensionNameInput(name string) error {
	if name == "" {
		return errors.New("extension name must not be empty")
	}
	if !extensionNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid extension name %q: use UpperCamelCase with a vendor prefix, letters and digits only (e.g. SwagBasicExample)", name)
	}

	return nil
}

// shopwarePluginBaseClass is the class every Shopware plugin class has to extend.
const shopwarePluginBaseClass = `Shopware\Core\Framework\Plugin`

// phpNamespaceRegexp matches the namespace declaration of a PHP file: `namespace Swag\BasicExample;`.
var phpNamespaceRegexp = regexp.MustCompile(`(?m)^namespace\s+([^;]+);`)

// phpClassRegexp matches a class declaration and its optional parent: `class SwagBasicExample extends Plugin`.
var phpClassRegexp = regexp.MustCompile(`(?m)^(?:final\s+|abstract\s+)?class\s+(\w+)(?:\s+extends\s+([\w\\]+))?`)

// validatePluginInstallable checks that a created plugin has everything Shopware needs
// to discover and install it:
//
//   - composer.json names a plugin class, requires shopware/core, has an English
//     label and autoloads the namespace of the plugin class.
//   - the plugin class file exists where the autoload entry points to and declares
//     the namespace, the class name and the base class composer.json promises.
//
// That composer.json exists, is valid JSON and has the type shopware-platform-plugin
// is already guaranteed by GetExtensionByFolder returning a PlatformPlugin.
//
// Every problem found is added to check, so the user sees all of them at once.
func validatePluginInstallable(plugin *PlatformPlugin, check validation.Check) {
	// The plugin class tells Shopware which class to load, so it is the value
	// everything else is compared against.
	pluginClass := plugin.Composer.Extra.ShopwarePluginClass
	namespace, className := splitPHPClass(pluginClass)
	if namespace == "" || className == "" {
		addInstallableError(check, "composer.json", "installable.plugin-class",
			fmt.Sprintf("extra.shopware-plugin-class must be a full class name like %s, got %q", `Swag\BasicExample\SwagBasicExample`, pluginClass))
		return
	}

	if _, err := plugin.GetShopwareVersionConstraint(); err != nil {
		addInstallableError(check, "composer.json", "installable.shopware-core",
			"require.shopware/core must be a valid version constraint: "+err.Error())
	}

	if plugin.Composer.Extra.Label["en-GB"] == "" {
		addInstallableError(check, "composer.json", "installable.label",
			"extra.label must contain a label for en-GB")
	}

	// The technical name of a plugin is its directory name, Shopware expects the
	// plugin class to have the same name.
	technicalName := filepath.Base(plugin.GetPath())
	if technicalName != className {
		addInstallableError(check, "composer.json", "installable.technical-name",
			fmt.Sprintf("extra.shopware-plugin-class must end with the directory name %q, got %q", technicalName, className))
	}

	validatePluginNameDerivation(technicalName, plugin.Composer.Name, namespace, check)

	classFile, found := pluginClassFile(plugin.Composer, namespace, className)
	if !found {
		addInstallableError(check, "composer.json", "installable.autoload",
			fmt.Sprintf(`autoload must map the namespace "%s\\" to a directory via psr-4 or psr-0`, namespace))
		return
	}

	validatePluginClassFile(plugin.GetPath(), classFile, namespace, className, check)
}

// validatePluginNameDerivation checks that composer.json still contains the values the
// scaffolding derives from the technical name. It catches a broken derivation, for
// example a namespace that lost a backslash while being written into JSON.
func validatePluginNameDerivation(technicalName, composerName, namespace string, check validation.Check) {
	if expected := scaffolding.DeriveComposerName(technicalName); composerName != expected {
		addInstallableError(check, "composer.json", "installable.composer-name",
			fmt.Sprintf("name must be %q for plugin %s, got %q", expected, technicalName, composerName))
	}

	if expected := scaffolding.DeriveNamespace(technicalName); namespace != expected {
		addInstallableError(check, "composer.json", "installable.namespace",
			fmt.Sprintf("namespace of extra.shopware-plugin-class must be %q for plugin %s, got %q", expected, technicalName, namespace))
	}
}

// validatePluginClassFile checks that the plugin class file exists and declares the
// namespace, class name and base class composer.json promises.
func validatePluginClassFile(extensionDir, classFile, namespace, className string, check validation.Check) {
	content, err := os.ReadFile(filepath.Join(extensionDir, classFile))
	if err != nil {
		addInstallableError(check, classFile, "installable.plugin-class-file",
			"plugin class file could not be read: "+err.Error())
		return
	}

	php := string(content)

	if declared := firstSubmatch(phpNamespaceRegexp, php); declared != namespace {
		addInstallableError(check, classFile, "installable.plugin-class-namespace",
			fmt.Sprintf("file must declare namespace %q, got %q", namespace, declared))
	}

	class := phpClassRegexp.FindStringSubmatch(php)
	if class == nil {
		addInstallableError(check, classFile, "installable.plugin-class-file",
			fmt.Sprintf("file must declare class %s", className))
		return
	}

	if class[1] != className {
		addInstallableError(check, classFile, "installable.plugin-class-file",
			fmt.Sprintf("file must declare class %q, got %q", className, class[1]))
	}

	if !extendsShopwarePlugin(class[2], php) {
		addInstallableError(check, classFile, "installable.plugin-base-class",
			fmt.Sprintf(`class %s must extend %s`, class[1], shopwarePluginBaseClass))
	}
}

// pluginClassFile returns the path of the plugin class file relative to the extension
// directory. PSR-4 maps the namespace prefix directly onto a directory, PSR-0 keeps
// the namespace as sub directories inside it.
func pluginClassFile(composer PlatformComposerJson, namespace, className string) (string, bool) {
	prefix := namespace + `\`
	fileName := className + ".php"

	if dir, ok := composer.Autoload.Psr4[prefix]; ok {
		return filepath.Join(dir, fileName), true
	}

	if dir, ok := composer.Autoload.Psr0[prefix]; ok {
		return filepath.Join(dir, filepath.Join(strings.Split(namespace, `\`)...), fileName), true
	}

	return "", false
}

// splitPHPClass splits a full class name like Swag\BasicExample\SwagBasicExample into
// its namespace (Swag\BasicExample) and its class name (SwagBasicExample). Both are
// empty when one of the parts is missing.
func splitPHPClass(fullClassName string) (namespace, className string) {
	parts := strings.Split(strings.TrimPrefix(fullClassName, `\`), `\`)
	if len(parts) < 2 || slices.Contains(parts, "") {
		return "", ""
	}

	return strings.Join(parts[:len(parts)-1], `\`), parts[len(parts)-1]
}

// extendsShopwarePlugin reports whether parent is Shopware's plugin base class, either
// written out in full or imported with a use statement.
func extendsShopwarePlugin(parent, php string) bool {
	parent = strings.TrimPrefix(parent, `\`)
	if parent == shopwarePluginBaseClass {
		return true
	}

	return parent == "Plugin" && strings.Contains(php, "use "+shopwarePluginBaseClass+";")
}

// firstSubmatch returns the first capture group of the first match, or an empty string
// when the pattern does not match at all.
func firstSubmatch(pattern *regexp.Regexp, content string) string {
	match := pattern.FindStringSubmatch(content)
	if match == nil {
		return ""
	}

	return strings.TrimSpace(match[1])
}

// addInstallableError records a problem that stops Shopware from installing the plugin.
func addInstallableError(check validation.Check, path, identifier, message string) {
	check.AddResult(validation.CheckResult{
		Path:       path,
		Identifier: identifier,
		Message:    message,
		Severity:   validation.SeverityError,
	})
}

/*
Use UpperCamelCase, which means that your plugin name must begin with a capital letter too.
Whenever possible, begin it with a company prefix to avoid duplicate names (e.g., SwagBasicExample).
Choose a name that describes your plugin as succinctly and clearly as possible.

INFO A vendor prefix is required if you plan to publish your plugin in the Shopware Community Store.
*/
