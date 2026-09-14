package extension

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/shopware/shopware-cli/internal/extension/scaffolding"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

// MinimumMakeShopwareVersion is the oldest Shopware release the generated code
// runs on. 6.7.13.0 replaced the custom field installer with a declarative
// Resources/config/custom-fields.xml, which the generators rely on.
const MinimumMakeShopwareVersion = "6.7.13.0"

// Generators returns the generators that can be run inside an existing plugin.
func Generators() []scaffolding.Generator {
	return scaffolding.Generators()
}

// Make runs a generator inside the plugin in the current directory.
func Make(ctx context.Context, generator scaffolding.Generator, args []string) error {
	logger := logging.FromContext(ctx)

	projectDir, err := shop.FindClosestShopwareProject(false)
	if err != nil {
		return err
	}

	if err := ensureMakeSupported(projectDir); err != nil {
		return err
	}

	plugin, err := currentPlugin(ctx)
	if err != nil {
		return err
	}

	result, err := generator.Run(plugin, args)
	if err != nil {
		return err
	}

	for _, path := range result.Created {
		logger.Infof("✓ created %s", path)
	}
	for _, path := range result.Updated {
		logger.Infof("✓ updated %s", path)
	}
	for _, path := range result.Skipped {
		logger.Infof("• skipped %s, it is already up to date", path)
	}

	return nil
}

// ensureMakeSupported fails when the project runs a Shopware release that does
// not understand the generated code.
func ensureMakeSupported(projectDir string) error {
	supported, err := shop.IsShopwareVersion(projectDir, ">="+MinimumMakeShopwareVersion)
	if err != nil {
		return fmt.Errorf("cannot determine the Shopware version of %s: %w", projectDir, err)
	}

	if !supported {
		return fmt.Errorf("the generators require Shopware %s or newer, %s uses an older release", MinimumMakeShopwareVersion, projectDir)
	}

	return nil
}

// currentPlugin describes the plugin the generators write into.
func currentPlugin(ctx context.Context) (scaffolding.PluginInfo, error) {
	dir, err := os.Getwd()
	if err != nil {
		return scaffolding.PluginInfo{}, err
	}

	ext, err := GetExtensionByFolder(ctx, dir)
	if err != nil {
		return scaffolding.PluginInfo{}, fmt.Errorf("the current directory is not an extension: %w", err)
	}

	plugin, ok := ext.(*PlatformPlugin)
	if !ok {
		return scaffolding.PluginInfo{}, fmt.Errorf("the generators only support plugins, %s is of type %s", dir, ext.GetType())
	}

	namespace, className, err := splitPluginClass(plugin.Composer.Extra.ShopwarePluginClass)
	if err != nil {
		return scaffolding.PluginInfo{}, err
	}

	return scaffolding.PluginInfo{Dir: dir, Namespace: namespace, ClassName: className}, nil
}

// splitPluginClass separates the namespace from the class name of the fully
// qualified plugin class, e.g. Swag\BasicExample\SwagBasicExample.
func splitPluginClass(pluginClass string) (namespace string, className string, err error) {
	separator := strings.LastIndex(pluginClass, `\`)
	if separator <= 0 || separator == len(pluginClass)-1 {
		return "", "", fmt.Errorf("composer.json needs a namespaced extra.shopware-plugin-class, got %q", pluginClass)
	}

	return pluginClass[:separator], pluginClass[separator+1:], nil
}
