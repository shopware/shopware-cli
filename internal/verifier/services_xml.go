package verifier

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/symfony"
	"github.com/shopware/shopware-cli/logging"
)

// ServicesXMLConverter converts deprecated Symfony services.xml files of
// platform plugins into services.yaml files.
type ServicesXMLConverter struct{}

func (ServicesXMLConverter) Name() string {
	return "services-xml"
}

func (ServicesXMLConverter) Check(ctx context.Context, check *Check, config ToolConfig) error {
	return nil
}

func (s ServicesXMLConverter) Fix(ctx context.Context, config ToolConfig) error {
	for _, configDir := range s.collectConfigDirs(ctx, config) {
		servicesXML := filepath.Join(configDir, "services.xml")

		if _, err := os.Stat(servicesXML); err != nil {
			continue
		}

		converted, err := symfony.ConvertServicesXMLFile(servicesXML)
		if err != nil {
			logging.FromContext(ctx).Warnf("Could not convert %s to YAML, keeping the XML file: %v", servicesXML, err)
			continue
		}

		for xmlPath, yamlPath := range converted {
			logging.FromContext(ctx).Infof("Converted %s to %s", xmlPath, yamlPath)
		}
	}

	return nil
}

func (ServicesXMLConverter) Format(ctx context.Context, config ToolConfig, dryRun bool) error {
	return nil
}

// collectConfigDirs returns the Resources/config directories of all platform
// plugins covered by the tool config. Apps cannot use the dependency injection
// container and bundles may load their container file explicitly by path, so
// only plugins (where Shopware picks services.xml or services.yaml
// automatically) are converted.
func (ServicesXMLConverter) collectConfigDirs(ctx context.Context, config ToolConfig) []string {
	dirs := []string{}

	addExtension := func(ext extension.Extension) {
		if ext.GetType() != extension.TypePlatformPlugin {
			return
		}

		for _, resourcesDir := range ext.GetResourcesDirs() {
			dirs = append(dirs, filepath.Join(resourcesDir, "config"))
		}

		if extCfg := ext.GetExtensionConfig(); extCfg != nil {
			for _, bundle := range extCfg.Build.ExtraBundles {
				dirs = append(dirs, filepath.Join(bundle.ResolvePath(ext.GetRootDir()), "Resources", "config"))
			}
		}
	}

	if config.Extension != nil {
		addExtension(config.Extension)

		return dirs
	}

	vendorDir := filepath.Join(config.RootDir, "vendor")

	for _, ext := range extension.FindExtensionsFromProject(logging.DisableLogger(ctx), config.RootDir, true) {
		rootDir := ext.GetRootDir()
		if resolvedPath, err := filepath.EvalSymlinks(rootDir); err == nil {
			rootDir = resolvedPath
		}

		if strings.HasPrefix(rootDir, vendorDir) {
			continue
		}

		addExtension(ext)
	}

	return dirs
}

func init() {
	AddTool(ServicesXMLConverter{})
}
