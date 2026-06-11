package symfony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// convertibleXMLConfig is a parsed Symfony XML configuration file that can be
// converted to YAML and may reference other files through imports.
type convertibleXMLConfig interface {
	// importReferences returns the imports, so the planner can rewrite
	// resources pointing to other converted XML files.
	importReferences() []xmlImportReference
	toYAML() ([]byte, error)
}

type xmlImportReference struct {
	resource   *string
	loaderType *string
}

func (c *Container) importReferences() []xmlImportReference {
	references := []xmlImportReference{}

	importLists := []*xmlImports{c.Imports}
	for i := range c.When {
		importLists = append(importLists, c.When[i].Imports)
	}

	for _, imports := range importLists {
		if imports == nil {
			continue
		}

		for i := range imports.Imports {
			references = append(references, xmlImportReference{
				resource:   &imports.Imports[i].Resource,
				loaderType: &imports.Imports[i].Type,
			})
		}
	}

	return references
}

func (c *Container) toYAML() ([]byte, error) {
	return ConvertContainerToYAML(c)
}

func (r *Routes) importReferences() []xmlImportReference {
	return routeItemImportReferences(r.Items)
}

func routeItemImportReferences(items []xmlRouteItem) []xmlImportReference {
	references := []xmlImportReference{}

	for i := range items {
		if items[i].Import != nil {
			references = append(references, xmlImportReference{
				resource:   &items[i].Import.Resource,
				loaderType: &items[i].Import.Type,
			})
		}

		if items[i].When != nil {
			references = append(references, routeItemImportReferences(items[i].When.Items)...)
		}
	}

	return references
}

func (r *Routes) toYAML() ([]byte, error) {
	return ConvertRoutesToYAML(r)
}

// ConvertServicesXMLFile converts a Symfony services.xml file and any local
// XML files referenced through <imports> into the YAML format. The YAML files
// are written next to the XML files and the XML files are removed afterwards.
// Nothing is written when one of the involved files cannot be converted.
// It returns a map of the XML files to the created YAML files.
func ConvertServicesXMLFile(path string) (map[string]string, error) {
	return convertXMLConfigFile(path, func(content []byte) (convertibleXMLConfig, error) {
		return ParseServicesXML(content)
	})
}

// ConvertRoutesXMLFile converts a Symfony routes.xml file and any local XML
// files referenced through imports, with the same guarantees as
// ConvertServicesXMLFile.
func ConvertRoutesXMLFile(path string) (map[string]string, error) {
	return convertXMLConfigFile(path, func(content []byte) (convertibleXMLConfig, error) {
		return ParseRoutesXML(content)
	})
}

func convertXMLConfigFile(path string, parse func([]byte) (convertibleXMLConfig, error)) (map[string]string, error) {
	contents := map[string][]byte{}
	targets := map[string]string{}

	if err := planConversion(path, parse, contents, targets); err != nil {
		return nil, err
	}

	for xmlPath, target := range targets {
		if err := os.WriteFile(target, contents[xmlPath], 0o644); err != nil {
			return nil, err
		}
	}

	for xmlPath := range targets {
		if err := os.Remove(xmlPath); err != nil {
			return nil, err
		}
	}

	return targets, nil
}

func planConversion(path string, parse func([]byte) (convertibleXMLConfig, error), contents map[string][]byte, targets map[string]string) error {
	if _, planned := targets[path]; planned {
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	config, err := parse(content)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	withoutExtension := strings.TrimSuffix(path, filepath.Ext(path))
	target := withoutExtension + ".yaml"

	for _, existing := range []string{target, withoutExtension + ".yml"} {
		if _, err := os.Stat(existing); err == nil {
			return fmt.Errorf("cannot convert %s: %s exists already", path, existing)
		}
	}

	targets[path] = target

	// Convert local XML files referenced by imports as well and rewrite the
	// import to the new YAML file.
	for _, reference := range config.importReferences() {
		resource := *reference.resource

		if !strings.HasSuffix(strings.ToLower(resource), ".xml") || strings.ContainsAny(resource, "*?[{") || filepath.IsAbs(resource) {
			continue
		}

		importPath := filepath.Join(filepath.Dir(path), filepath.FromSlash(resource))
		if _, err := os.Stat(importPath); err != nil {
			continue
		}

		if err := planConversion(importPath, parse, contents, targets); err != nil {
			return err
		}

		*reference.resource = resource[:len(resource)-len(".xml")] + ".yaml"

		if *reference.loaderType == "xml" {
			*reference.loaderType = "yaml"
		}
	}

	converted, err := config.toYAML()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	contents[path] = converted

	return nil
}
