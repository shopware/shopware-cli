package verifier

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/symfony"
	"github.com/shopware/shopware-cli/internal/validation"
)

// SymfonyXMLConverter converts deprecated Symfony XML configuration files
// (services.xml and routes.xml) of platform plugins into their YAML
// counterparts.
type SymfonyXMLConverter struct{}

func (SymfonyXMLConverter) Name() string {
	return "symfony-xml"
}

func (s SymfonyXMLConverter) Fix(_ context.Context, config ToolConfig) ToolResult {
	result := ToolResult{}
	found := false
	changed := 0
	conversions := []struct {
		fileName string
		convert  func(string) (map[string]string, error)
	}{
		{"services.xml", symfony.ConvertServicesXMLFile},
		{"routes.xml", symfony.ConvertRoutesXMLFile},
	}

	for _, configDir := range s.collectConfigDirs(config) {
		for _, conversion := range conversions {
			xmlPath := filepath.Join(configDir, conversion.fileName)

			if _, err := os.Stat(xmlPath); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					result.Err = errors.Join(result.Err, fmt.Errorf("cannot read XML configuration: %w", err))
				}
				continue
			}
			found = true

			converted, err := conversion.convert(xmlPath)
			if err != nil {
				var pathErr *os.PathError
				if errors.As(err, &pathErr) {
					result.Err = errors.Join(result.Err, fmt.Errorf("cannot convert XML configuration: %w", err))
				} else {
					result.Findings = append(result.Findings, validation.CheckResult{
						Path: validation.NormalizeSourcePath(xmlPath, config.RootDir), Severity: validation.SeverityWarning,
						Identifier: "symfony-xml/unsupported", Message: fmt.Sprintf("Cannot convert XML to YAML; keeping the XML file: %v", err),
					})
				}
				continue
			}

			changed += len(converted)
		}
	}

	if !found && result.Err == nil {
		result.SkipReason = "no XML configuration files"
	}
	if found {
		result.Note = fileChangeNote(changed, "converted")
	}
	return result
}

// collectConfigDirs returns the Resources/config directories of all platform
// plugins covered by the tool config. Apps cannot use the dependency injection
// container and bundles may load their configuration files explicitly by
// path, so only plugins (where Shopware picks the XML or YAML variant
// automatically) are converted.
func (SymfonyXMLConverter) collectConfigDirs(config ToolConfig) []string {
	dirs := []string{}

	for _, ext := range config.Extensions {
		if ext.GetType() != extension.TypePlatformPlugin {
			continue
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

	return dirs
}

func init() {
	AddTool(SymfonyXMLConverter{})
}
