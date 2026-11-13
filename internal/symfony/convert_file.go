package symfony

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

// ConvertServicesXMLFiles converts Symfony services XML files and any local
// XML files referenced through <imports> into the YAML format. The YAML files
// are written next to the XML files and the XML files are removed afterwards.
// Nothing is written when one of the involved files cannot be converted.
// It returns a map of the XML files to the created YAML files.
func ConvertServicesXMLFiles(ctx context.Context, paths ...string) (map[string]string, error) {
	return convertXMLConfigFiles(ctx, paths, parseServicesConfig, false)
}

// ConvertRoutesXMLFiles converts Symfony routing XML files and any local XML
// files referenced through imports, with the same guarantees as
// ConvertServicesXMLFiles.
func ConvertRoutesXMLFiles(ctx context.Context, paths ...string) (map[string]string, error) {
	return convertXMLConfigFiles(ctx, paths, parseRoutesConfig, false)
}

// PlanServicesXMLFiles validates conversion of services files and their local
// XML imports, returning the planned XML-to-YAML paths without changing any files.
func PlanServicesXMLFiles(ctx context.Context, paths ...string) (map[string]string, error) {
	return convertXMLConfigFiles(ctx, paths, parseServicesConfig, true)
}

// PlanRoutesXMLFiles validates conversion of routing files and their local XML
// imports, returning the planned XML-to-YAML paths without changing any files.
func PlanRoutesXMLFiles(ctx context.Context, paths ...string) (map[string]string, error) {
	return convertXMLConfigFiles(ctx, paths, parseRoutesConfig, true)
}

func parseServicesConfig(content []byte) (convertibleXMLConfig, error) {
	return ParseServicesXML(content)
}

func parseRoutesConfig(content []byte) (convertibleXMLConfig, error) {
	return ParseRoutesXML(content)
}

// The namespaced root elements Symfony requires for services and routing XML
// files. It refuses to load a file without the namespace.
var (
	servicesXMLRoot = xml.Name{Space: "http://symfony.com/schema/dic/services", Local: "container"}
	routesXMLRoot   = xml.Name{Space: "http://symfony.com/schema/routing", Local: "routes"}
)

// FindXMLConfigFiles returns the services and routing XML files in the
// configuration directory dir besides services.xml and routes.xml. They can be
// loaded by Shopware itself or imported from PHP or YAML configuration, so the
// files are recognized by their root element. Files matched by a glob import
// are left out, as glob imports keep loading XML files, and so are the files
// they import, as their imports are not rewritten.
func FindXMLConfigFiles(ctx context.Context, dir string) (services []string, routes []string, err error) {
	servicesPath := filepath.Join(dir, "services.xml")
	routesPath := filepath.Join(dir, "routes.xml")
	imports := map[string][]string{}
	pending := []string{}

	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if path == dir && errors.Is(err, fs.ErrNotExist) {
			return fs.SkipAll
		}
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".xml") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			// The conversion of services.xml and routes.xml reports it.
			if path == servicesPath || path == routesPath {
				return nil
			}
			return err
		}

		parse := parseServicesConfig
		switch root := xmlRootName(content); {
		case path == servicesPath:
		case path == routesPath:
			parse = parseRoutesConfig
		case root == servicesXMLRoot:
			services = append(services, path)
		case root == routesXMLRoot:
			parse = parseRoutesConfig
			routes = append(routes, path)
		default:
			return nil
		}

		// A file that cannot be parsed is reported by its conversion.
		if config, err := parse(content); err == nil {
			imported, globbed := xmlImportMatches(path, config)
			imports[path] = imported
			pending = append(pending, globbed...)
		}

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	retained := map[string]struct{}{}
	for len(pending) > 0 {
		path := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if _, ok := retained[path]; ok {
			continue
		}
		retained[path] = struct{}{}
		pending = append(pending, imports[path]...)
	}

	isRetained := func(path string) bool {
		_, ok := retained[path]
		return ok
	}

	return slices.DeleteFunc(services, isRetained), slices.DeleteFunc(routes, isRetained), nil
}

// xmlImportMatches returns the existing local XML files the file at path
// imports, and separately those it imports through a glob pattern.
func xmlImportMatches(path string, config convertibleXMLConfig) (imported []string, globbed []string) {
	for _, reference := range config.importReferences() {
		resource := *reference.resource
		if !strings.HasSuffix(strings.ToLower(resource), ".xml") || filepath.IsAbs(resource) {
			continue
		}

		// A plain path matches itself when the file exists. A malformed
		// pattern matches nothing.
		matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), filepath.FromSlash(resource)))
		imported = append(imported, matches...)
		if strings.ContainsAny(resource, "*?[{") {
			globbed = append(globbed, matches...)
		}
	}

	return imported, globbed
}

// xmlRootName returns the namespaced name of the root element, or the zero
// name when content is not well-formed up to it.
func xmlRootName(content []byte) xml.Name {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	for {
		token, err := decoder.Token()
		if err != nil {
			return xml.Name{}
		}
		if start, ok := token.(xml.StartElement); ok {
			return start.Name
		}
	}
}

func convertXMLConfigFiles(
	ctx context.Context,
	paths []string,
	parse func([]byte) (convertibleXMLConfig, error),
	dryRun bool,
) (map[string]string, error) {
	contents := map[string][]byte{}
	targets := map[string]string{}

	// Plan all files together, so a file imported more than once or also
	// passed itself is converted once and every import of it is rewritten.
	for _, path := range paths {
		if err := planConversion(ctx, path, parse, contents, targets); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if dryRun {
		return targets, nil
	}

	// Once writes begin, finish this conversion so cancellation cannot leave
	// imports split between XML and YAML. Stop at the next conversion boundary.
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

func planConversion(ctx context.Context, path string, parse func([]byte) (convertibleXMLConfig, error), contents map[string][]byte, targets map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, planned := targets[path]; planned {
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	config, err := parse(content)
	if err != nil {
		kind := XMLConversionUnsupported
		var syntaxError *xml.SyntaxError
		if errors.As(err, &syntaxError) {
			kind = XMLConversionInvalidXML
		}
		return &XMLConversionError{Path: path, Kind: kind, Err: err}
	}

	withoutExtension := strings.TrimSuffix(path, filepath.Ext(path))
	target := withoutExtension + ".yaml"

	for _, existing := range []string{target, withoutExtension + ".yml"} {
		if _, err := os.Stat(existing); err == nil {
			return &XMLConversionError{Path: path, Kind: XMLConversionTargetExists, Err: fmt.Errorf("%s exists already", existing)}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	for source, planned := range targets {
		if planned == target {
			return &XMLConversionError{Path: path, Kind: XMLConversionTargetExists, Err: fmt.Errorf("%s is converted to %s as well", source, target)}
		}
	}

	targets[path] = target

	// Convert local XML files referenced by imports as well and rewrite the
	// import to the new YAML file.
	for _, reference := range config.importReferences() {
		if err := ctx.Err(); err != nil {
			return err
		}
		resource := *reference.resource

		if !strings.HasSuffix(strings.ToLower(resource), ".xml") || strings.ContainsAny(resource, "*?[{") || filepath.IsAbs(resource) {
			continue
		}

		importPath := filepath.Join(filepath.Dir(path), filepath.FromSlash(resource))
		if _, err := os.Stat(importPath); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}

		if err := planConversion(ctx, importPath, parse, contents, targets); err != nil {
			return err
		}

		*reference.resource = resource[:len(resource)-len(".xml")] + ".yaml"

		if *reference.loaderType == "xml" {
			*reference.loaderType = "yaml"
		}
	}

	converted, err := config.toYAML()
	if err != nil {
		return &XMLConversionError{Path: path, Kind: XMLConversionUnsupported, Err: err}
	}

	contents[path] = converted

	return nil
}
