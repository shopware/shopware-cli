package verifier

import (
	"context"
	"encoding/xml"
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

func (s SymfonyXMLConverter) Fix(ctx context.Context, config ToolConfig, execution *ToolExecution, dryRun bool) ToolSummary {
	found := false
	changed := 0
	failed := false
	pendingFiles := make(map[string]struct{})
	conversions := []struct {
		fileName string
		convert  func(context.Context, string) (map[string]string, error)
		plan     func(context.Context, string) (map[string]string, error)
	}{
		{"services.xml", symfony.ConvertServicesXMLFile, symfony.PlanServicesXMLFile},
		{"routes.xml", symfony.ConvertRoutesXMLFile, symfony.PlanRoutesXMLFile},
	}

	for _, configDir := range s.collectConfigDirs(config) {
		for _, conversion := range conversions {
			if ctx.Err() != nil {
				return ToolSummary{}
			}
			xmlPath := filepath.Join(configDir, conversion.fileName)

			if _, err := os.Stat(xmlPath); err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					failed = true
					execution.Fail(&ToolError{Err: fmt.Errorf("cannot read XML configuration: %w", err)})
				}
				continue
			}
			found = true

			convert := conversion.convert
			if dryRun {
				convert = conversion.plan
			}

			converted, err := convert(ctx, xmlPath)
			if err != nil {
				if finding, ok := symfonyXMLFinding(err, config.RootDir); ok {
					execution.AddFindings(finding)
				} else {
					failed = true
					execution.Fail(&ToolError{Err: fmt.Errorf("cannot convert XML configuration: %w", err)})
				}
				continue
			}

			if !dryRun {
				changed += len(converted)
				continue
			}

			for source := range converted {
				filePath := validation.NormalizeSourcePath(source, config.RootDir)
				if _, exists := pendingFiles[filePath]; exists {
					continue
				}
				pendingFiles[filePath] = struct{}{}
				execution.AddFindings(validation.CheckResult{
					Path:       filePath,
					Severity:   validation.SeverityError,
					Identifier: "symfony-xml/conversion",
					Message:    "XML configuration can be converted to YAML. Run fix without dry-run to apply it.",
				})
			}
		}
	}

	if !found && !failed {
		return ToolSummary{SkipReason: "no XML configuration files"}
	}
	if dryRun {
		return ToolSummary{}
	}

	// I/O failures can leave partial writes. Avoid claiming no changes when
	// conversion did not return a completed result.
	if failed && changed == 0 {
		return ToolSummary{}
	}

	return ToolSummary{Note: fileChangeNote(changed, "converted")}
}

// symfonyXMLFinding maps source blockers to findings. Filesystem failures remain
// execution errors because conversion may have left partially written files.
func symfonyXMLFinding(err error, root string) (validation.CheckResult, bool) {
	var conversionErr *symfony.XMLConversionError
	if !errors.As(err, &conversionErr) {
		return validation.CheckResult{}, false
	}

	line := 0
	var syntaxErr *xml.SyntaxError
	if errors.As(err, &syntaxErr) {
		line = syntaxErr.Line
	}

	// Retain the original root alias while normalizing embedded source paths.
	message := validation.StripRootFromText(
		fmt.Sprintf("Cannot convert XML to YAML; keeping the XML file: %v", err), root,
	)

	return validation.CheckResult{
		Path:       validation.NormalizeSourcePath(conversionErr.Path, root),
		Line:       line,
		Severity:   validation.SeverityError,
		Identifier: "symfony-xml/" + string(conversionErr.Kind),
		Message:    message,
		Tip:        "Resolve the XML issue or convert the configuration manually, then rerun fix.",
	}, true
}

// collectConfigDirs returns the unique Resources/config directories of all platform
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

	seen := make(map[string]struct{})
	unique := make([]string, 0, len(dirs))
	for _, directory := range dirs {
		if _, exists := seen[directory]; exists {
			continue
		}
		seen[directory] = struct{}{}
		unique = append(unique, directory)
	}

	return unique
}

func init() {
	AddTool(SymfonyXMLConverter{})
}
