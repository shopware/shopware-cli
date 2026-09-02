package extension

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/internal/extension/scaffolding"
	"github.com/shopware/shopware-cli/internal/validation"
)

const shopwarePluginBaseClass = `Shopware\Core\Framework\Plugin`

var (
	phpNamespaceRegexp = regexp.MustCompile(`(?m)^namespace\s+([^;]+);`)
	phpClassRegexp     = regexp.MustCompile(`(?m)^(?:final\s+|abstract\s+)?class\s+(\w+)(?:\s+extends\s+([\w\\]+))?`)
)

// validateCreatedExtension reloads the generated files before validation. This
// checks what was written to disk rather than trusting the template input.
func validateCreatedExtension(ctx context.Context, extensionDir string) error {
	ext, err := GetExtensionByFolder(ctx, extensionDir)
	if err != nil {
		return fmt.Errorf("load extension: %w", err)
	}

	plugin, ok := ext.(*PlatformPlugin)
	if !ok {
		return fmt.Errorf("%s is not a %s", extensionDir, ComposerTypePlugin)
	}

	check := &installabilityCheck{}
	validatePluginInstallable(plugin, check)
	if check.HasErrors() {
		return installabilityError{results: check.GetResults()}
	}

	return nil
}

// validatePluginInstallable checks the metadata, autoload mapping, and PHP
// class Shopware needs to discover and install a plugin. Loading the plugin
// above already verifies that composer.json is valid and has the right type.
func validatePluginInstallable(plugin *PlatformPlugin, check validation.Check) {
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

	technicalName := filepath.Base(plugin.GetPath())
	if technicalName != className {
		addInstallableError(check, "composer.json", "installable.technical-name",
			fmt.Sprintf("extra.shopware-plugin-class must end with the directory name %q, got %q", technicalName, className))
	}

	validatePluginNameDerivation(technicalName, plugin.Composer.Name, namespace, check)

	classFile, found := pluginClassFile(plugin.Composer, namespace, className)
	if !found {
		addInstallableError(check, "composer.json", "installable.autoload",
			fmt.Sprintf(`autoload must map the namespace "%s\\" through psr-4 or psr-0`, namespace))
		return
	}

	validatePluginClassFile(plugin.GetPath(), classFile, namespace, className, check)
}

func validatePluginNameDerivation(technicalName, composerName, namespace string, check validation.Check) {
	if expected := scaffolding.DeriveComposerName(technicalName); composerName != expected {
		addInstallableError(check, "composer.json", "installable.composer-name",
			fmt.Sprintf("name must be %q for plugin %s, got %q", expected, technicalName, composerName))
	}

	if expected := scaffolding.DeriveNamespace(technicalName); namespace != expected {
		addInstallableError(check, "composer.json", "installable.namespace",
			fmt.Sprintf("plugin namespace must be %q for plugin %s, got %q", expected, technicalName, namespace))
	}
}

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
			fmt.Sprintf("class %s must extend %s", class[1], shopwarePluginBaseClass))
	}
}

// pluginClassFile resolves the class file using the mappings generated in
// composer.json. PSR-4 strips the namespace prefix; PSR-0 keeps the namespace
// as directories below its mapped path.
func pluginClassFile(composer PlatformComposerJson, namespace, className string) (string, bool) {
	prefix := namespace + `\`
	fileName := className + ".php"

	if dir, ok := composer.Autoload.Psr4[prefix]; ok {
		return filepath.Join(dir, fileName), true
	}
	if dir, ok := composer.Autoload.Psr0[prefix]; ok {
		namespacePath := filepath.Join(strings.Split(namespace, `\`)...)
		return filepath.Join(dir, namespacePath, fileName), true
	}

	return "", false
}

func splitPHPClass(fullClassName string) (namespace, className string) {
	parts := strings.Split(strings.TrimPrefix(fullClassName, `\`), `\`)
	if len(parts) < 2 || slices.Contains(parts, "") {
		return "", ""
	}

	return strings.Join(parts[:len(parts)-1], `\`), parts[len(parts)-1]
}

func extendsShopwarePlugin(parent, php string) bool {
	parent = strings.TrimPrefix(parent, `\`)
	if parent == shopwarePluginBaseClass {
		return true
	}

	return parent == "Plugin" && strings.Contains(php, "use "+shopwarePluginBaseClass+";")
}

func firstSubmatch(pattern *regexp.Regexp, content string) string {
	match := pattern.FindStringSubmatch(content)
	if match == nil {
		return ""
	}

	return strings.TrimSpace(match[1])
}

func addInstallableError(check validation.Check, path, identifier, message string) {
	check.AddResult(validation.CheckResult{
		Path:       path,
		Identifier: identifier,
		Message:    message,
		Severity:   validation.SeverityError,
	})
}

type installabilityCheck struct {
	results []validation.CheckResult
}

func (c *installabilityCheck) AddResult(result validation.CheckResult) {
	c.results = append(c.results, result)
}

func (c *installabilityCheck) GetResults() []validation.CheckResult {
	return c.results
}

func (c *installabilityCheck) HasErrors() bool {
	for _, result := range c.results {
		if result.Severity == validation.SeverityError {
			return true
		}
	}

	return false
}

func (c *installabilityCheck) RemoveByIdentifier([]validation.ToolConfigIgnore) validation.Check {
	return c
}

type installabilityError struct {
	results []validation.CheckResult
}

func (e installabilityError) Error() string {
	messages := make([]string, 0, len(e.results))
	for _, result := range e.results {
		messages = append(messages, fmt.Sprintf("%s [%s]: %s", result.Path, result.Identifier, result.Message))
	}

	return strings.Join(messages, "; ")
}
