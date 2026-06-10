package symfony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConvertServicesXMLFile converts a Symfony services.xml file and any local
// XML files referenced through <imports> into the YAML format. The YAML files
// are written next to the XML files and the XML files are removed afterwards.
// Nothing is written when one of the involved files cannot be converted.
// It returns a map of the XML files to the created YAML files.
func ConvertServicesXMLFile(path string) (map[string]string, error) {
	contents := map[string][]byte{}
	targets := map[string]string{}

	if err := planConversion(path, contents, targets); err != nil {
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

func planConversion(path string, contents map[string][]byte, targets map[string]string) error {
	if _, planned := targets[path]; planned {
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	container, err := ParseServicesXML(content)
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

	if err := planImportConversions(path, container.Imports, contents, targets); err != nil {
		return err
	}

	for i := range container.When {
		if err := planImportConversions(path, container.When[i].Imports, contents, targets); err != nil {
			return err
		}
	}

	converted, err := ConvertContainerToYAML(container)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	contents[path] = converted

	return nil
}

// planImportConversions converts local XML files referenced by <import>
// elements as well and rewrites the import to the new YAML file.
func planImportConversions(path string, imports *xmlImports, contents map[string][]byte, targets map[string]string) error {
	if imports == nil {
		return nil
	}

	for i := range imports.Imports {
		resource := imports.Imports[i].Resource

		if !strings.HasSuffix(strings.ToLower(resource), ".xml") || strings.ContainsAny(resource, "*?[{") || filepath.IsAbs(resource) {
			continue
		}

		importPath := filepath.Join(filepath.Dir(path), filepath.FromSlash(resource))
		if _, err := os.Stat(importPath); err != nil {
			continue
		}

		if err := planConversion(importPath, contents, targets); err != nil {
			return err
		}

		imports.Imports[i].Resource = resource[:len(resource)-len(".xml")] + ".yaml"
	}

	return nil
}
