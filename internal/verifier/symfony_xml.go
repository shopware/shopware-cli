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

// SymfonyXMLConverter converts deprecated Symfony XML configuration files
// (services.xml and routes.xml) of platform plugins into their YAML
// counterparts.
type SymfonyXMLConverter struct{}

func (SymfonyXMLConverter) Name() string {
	return "symfony-xml"
}

func (SymfonyXMLConverter) Check(ctx context.Context, check *Check, config ToolConfig) error {
	return nil
}

func (s SymfonyXMLConverter) Fix(ctx context.Context, config ToolConfig) error {
	conversions := []struct {
		fileName string
		convert  func(string) (map[string]string, error)
	}{
		{"services.xml", symfony.ConvertServicesXMLFile},
		{"routes.xml", symfony.ConvertRoutesXMLFile},
	}

	for _, configDir := range s.collectConfigDirs(ctx, config) {
		for _, conversion := range conversions {
			xmlPath := filepath.Join(configDir, conversion.fileName)

			if _, err := os.Stat(xmlPath); err != nil {
				continue
			}

			converted, err := conversion.convert(xmlPath)
			if err != nil {
				logging.FromContext(ctx).Warnf("Could not convert %s to YAML, keeping the XML file: %v", xmlPath, err)
				continue
			}

			for from, to := range converted {
				logging.FromContext(ctx).Infof("Converted %s to %s", from, to)
			}
		}
	}

	return nil
}

func (SymfonyXMLConverter) Format(ctx context.Context, config ToolConfig, dryRun bool) error {
	return nil
}

// collectConfigDirs returns the Resources/config directories of all platform
// plugins covered by the tool config. Apps cannot use the dependency injection
// container and bundles may load their configuration files explicitly by
// path, so only plugins (where Shopware picks the XML or YAML variant
// automatically) are converted.
func (SymfonyXMLConverter) collectConfigDirs(ctx context.Context, config ToolConfig) []string {
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
	AddTool(SymfonyXMLConverter{})
}
