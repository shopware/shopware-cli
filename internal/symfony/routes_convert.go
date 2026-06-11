package symfony

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	listSplitRegexp         = regexp.MustCompile(`[\s,|]+`)
	importKeySanitizeRegexp = regexp.MustCompile(`[^a-z0-9]+`)
)

// ConvertRoutesToYAML converts a parsed routes.xml file into the equivalent
// routes.yaml content. Like the services conversion it refuses everything
// that cannot be expressed safely in YAML.
func ConvertRoutesToYAML(routes *Routes) ([]byte, error) {
	root := newMapping()
	usedKeys := map[string]bool{}

	if err := appendRouteItems(root, routes.Items, usedKeys, true); err != nil {
		return nil, err
	}

	if len(root.Content) == 0 {
		return []byte{}, nil
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(4)

	if err := encoder.Encode(root); err != nil {
		return nil, err
	}

	if err := encoder.Close(); err != nil {
		return nil, err
	}

	return insertBlankLines(buf.Bytes()), nil
}

func appendRouteItems(target *yaml.Node, items []xmlRouteItem, usedKeys map[string]bool, allowWhen bool) error {
	for _, item := range items {
		switch item.Kind {
		case "route":
			key, node, err := routeToNode(item.Route)
			if err != nil {
				return err
			}

			if usedKeys[key] {
				return fmt.Errorf("route %q is defined multiple times", key)
			}
			usedKeys[key] = true

			mapPut(target, key, node)
		case "import":
			node, err := routeImportToNode(item.Import)
			if err != nil {
				return err
			}

			mapPut(target, routeImportKey(item.Import.Resource, usedKeys), node)
		case "when":
			if !allowWhen {
				return fmt.Errorf("<when> elements cannot be nested")
			}

			if item.When.Env == "" {
				return fmt.Errorf("<when> requires an env attribute")
			}

			key := "when@" + item.When.Env
			if usedKeys[key] {
				return fmt.Errorf("environment %q is configured multiple times", item.When.Env)
			}
			usedKeys[key] = true

			whenMapping := newMapping()
			if err := appendRouteItems(whenMapping, item.When.Items, map[string]bool{}, false); err != nil {
				return err
			}

			mapPut(target, key, whenMapping)
		default:
			return fmt.Errorf("unsupported element <%s> inside <routes>", item.Kind)
		}
	}

	return nil
}

// routeImportKey derives a readable, unique YAML key for an import, since
// XML imports are anonymous while the YAML format needs a mapping key.
func routeImportKey(resource string, usedKeys map[string]bool) string {
	sanitized := resource

	if globOffset := strings.IndexAny(sanitized, "*?[{"); globOffset >= 0 {
		sanitized = sanitized[:globOffset]
	}

	sanitized = strings.TrimSuffix(sanitized, filepath.Ext(sanitized))

	parts := []string{}
	for _, part := range strings.FieldsFunc(sanitized, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == "." || part == ".." {
			continue
		}

		part = importKeySanitizeRegexp.ReplaceAllString(strings.ToLower(part), "_")
		if part = strings.Trim(part, "_"); part != "" {
			parts = append(parts, part)
		}
	}

	key := strings.Join(parts, "_")
	if key == "" {
		key = "imported_routes"
	}

	if !usedKeys[key] {
		usedKeys[key] = true
		return key
	}

	for i := 2; ; i++ {
		candidate := key + "_" + strconv.Itoa(i)
		if !usedKeys[candidate] {
			usedKeys[candidate] = true
			return candidate
		}
	}
}

func routeToNode(route *xmlRoute) (string, *yaml.Node, error) {
	if route.ID == "" {
		return "", nil, fmt.Errorf("<route> requires an id attribute")
	}

	node, err := buildRouteNode(route)
	if err != nil {
		return "", nil, fmt.Errorf("route %q: %w", route.ID, err)
	}

	return route.ID, node, nil
}

func buildRouteNode(route *xmlRoute) (*yaml.Node, error) {
	if err := unknownElementError("route", route.Unknown); err != nil {
		return nil, err
	}

	if err := unknownAttributeError("route", route.UnknownAttrs); err != nil {
		return nil, err
	}

	mapping := newMapping()

	switch {
	case route.Path != "" && len(route.Paths) > 0:
		return nil, fmt.Errorf("must not have both a path attribute and <path> child elements")
	case route.Path != "":
		mapPut(mapping, "path", newString(route.Path))
	case len(route.Paths) > 0:
		node, err := localizedValuesToNode(route.Paths, "path")
		if err != nil {
			return nil, err
		}

		mapPut(mapping, "path", node)
	default:
		return nil, fmt.Errorf("requires a path attribute or <path> child elements")
	}

	if route.Controller != "" {
		for _, def := range route.Defaults {
			if def.Key == "_controller" {
				return nil, fmt.Errorf("must not specify both the controller attribute and the _controller default")
			}
		}

		mapPut(mapping, "controller", newString(route.Controller))
	}

	if err := appendSharedRouteConfig(mapping, sharedRouteConfig{
		host:         route.Host,
		hosts:        route.Hosts,
		schemes:      route.Schemes,
		methods:      route.Methods,
		locale:       route.Locale,
		format:       route.Format,
		utf8:         route.UTF8,
		stateless:    route.Stateless,
		defaults:     route.Defaults,
		requirements: route.Requirements,
		options:      route.Options,
	}); err != nil {
		return nil, err
	}

	if condition := strings.TrimSpace(route.Condition); condition != "" {
		mapPut(mapping, "condition", newString(condition))
	}

	return mapping, nil
}

func routeImportToNode(imp *xmlRouteImport) (*yaml.Node, error) {
	if err := unknownElementError("import", imp.Unknown); err != nil {
		return nil, err
	}

	if err := unknownAttributeError("import", imp.UnknownAttrs); err != nil {
		return nil, err
	}

	if imp.Resource == "" {
		return nil, fmt.Errorf("<import> requires a resource attribute")
	}

	mapping := newMapping()
	mapPut(mapping, "resource", newString(imp.Resource))

	if imp.Type != "" {
		mapPut(mapping, "type", newString(imp.Type))
	}

	switch {
	case imp.Prefix != "" && len(imp.Prefixes) > 0:
		return nil, fmt.Errorf("import %q: must not have both a prefix attribute and <prefix> child elements", imp.Resource)
	case imp.Prefix != "":
		mapPut(mapping, "prefix", newString(imp.Prefix))
	case len(imp.Prefixes) > 0:
		node, err := localizedValuesToNode(imp.Prefixes, "prefix")
		if err != nil {
			return nil, fmt.Errorf("import %q: %w", imp.Resource, err)
		}

		mapPut(mapping, "prefix", node)
	}

	if imp.NamePrefix != "" {
		mapPut(mapping, "name_prefix", newString(imp.NamePrefix))
	}

	if imp.ExcludeAttr != "" && len(imp.Excludes) > 0 {
		return nil, fmt.Errorf("import %q: mixes the exclude attribute with <exclude> elements", imp.Resource)
	}

	switch {
	case imp.ExcludeAttr != "":
		mapPut(mapping, "exclude", newString(imp.ExcludeAttr))
	case len(imp.Excludes) == 1:
		mapPut(mapping, "exclude", newString(imp.Excludes[0]))
	case len(imp.Excludes) > 1:
		excludes := newSequence()
		for _, exclude := range imp.Excludes {
			excludes.Content = append(excludes.Content, newString(exclude))
		}

		mapPut(mapping, "exclude", excludes)
	}

	if imp.TrailingSlashOnRoot != "" {
		mapPut(mapping, "trailing_slash_on_root", scalarValue(phpize(imp.TrailingSlashOnRoot)))
	}

	if err := appendSharedRouteConfig(mapping, sharedRouteConfig{
		host:         imp.Host,
		hosts:        imp.Hosts,
		schemes:      imp.Schemes,
		methods:      imp.Methods,
		locale:       imp.Locale,
		format:       imp.Format,
		utf8:         imp.UTF8,
		stateless:    imp.Stateless,
		defaults:     imp.Defaults,
		requirements: imp.Requirements,
		options:      imp.Options,
	}); err != nil {
		return nil, fmt.Errorf("import %q: %w", imp.Resource, err)
	}

	return mapping, nil
}

// sharedRouteConfig covers the configuration that <route> and <import>
// elements have in common.
type sharedRouteConfig struct {
	host         string
	hosts        []xmlLocalizedValue
	schemes      string
	methods      string
	locale       string
	format       string
	utf8         string
	stateless    string
	defaults     []xmlRouteDefault
	requirements []xmlKeyValue
	options      []xmlKeyValue
}

func appendSharedRouteConfig(mapping *yaml.Node, config sharedRouteConfig) error {
	switch {
	case config.host != "" && len(config.hosts) > 0:
		return fmt.Errorf("must not have both a host attribute and <host> child elements")
	case config.host != "":
		mapPut(mapping, "host", newString(config.host))
	case len(config.hosts) > 0:
		node, err := localizedValuesToNode(config.hosts, "host")
		if err != nil {
			return err
		}

		mapPut(mapping, "host", node)
	}

	if config.schemes != "" {
		mapPut(mapping, "schemes", listToNode(config.schemes))
	}

	if config.methods != "" {
		mapPut(mapping, "methods", listToNode(config.methods))
	}

	if len(config.defaults) > 0 {
		node, err := routeDefaultsToNode(config.defaults)
		if err != nil {
			return err
		}

		mapPut(mapping, "defaults", node)
	}

	if len(config.requirements) > 0 {
		node, err := keyValuesToNode(config.requirements, "requirement", func(value string) *yaml.Node {
			return newString(strings.TrimSpace(value))
		})
		if err != nil {
			return err
		}

		mapPut(mapping, "requirements", node)
	}

	if len(config.options) > 0 {
		node, err := keyValuesToNode(config.options, "option", func(value string) *yaml.Node {
			return scalarValue(phpize(value))
		})
		if err != nil {
			return err
		}

		mapPut(mapping, "options", node)
	}

	if config.locale != "" {
		mapPut(mapping, "locale", newString(config.locale))
	}

	if config.format != "" {
		mapPut(mapping, "format", newString(config.format))
	}

	if config.utf8 != "" {
		mapPut(mapping, "utf8", scalarValue(phpize(config.utf8)))
	}

	if config.stateless != "" {
		mapPut(mapping, "stateless", scalarValue(phpize(config.stateless)))
	}

	return nil
}

// listToNode splits schemes/methods lists like "GET|POST" or "GET, POST".
func listToNode(value string) *yaml.Node {
	sequence := newSequence()
	sequence.Style = yaml.FlowStyle

	for _, entry := range listSplitRegexp.Split(value, -1) {
		if entry == "" {
			continue
		}

		sequence.Content = append(sequence.Content, newString(entry))
	}

	return sequence
}

func localizedValuesToNode(values []xmlLocalizedValue, element string) (*yaml.Node, error) {
	mapping := newMapping()

	for _, value := range values {
		if value.Locale == "" {
			return nil, fmt.Errorf("<%s> child elements require a locale attribute", element)
		}

		mapPut(mapping, value.Locale, newString(strings.TrimSpace(value.Value)))
	}

	return mapping, nil
}

func keyValuesToNode(values []xmlKeyValue, element string, convert func(string) *yaml.Node) (*yaml.Node, error) {
	mapping := newMapping()

	for _, value := range values {
		if value.Key == "" {
			return nil, fmt.Errorf("<%s> requires a key attribute", element)
		}

		mapPut(mapping, value.Key, convert(value.Value))
	}

	return mapping, nil
}

func routeDefaultsToNode(defaults []xmlRouteDefault) (*yaml.Node, error) {
	mapping := newMapping()

	for _, def := range defaults {
		if def.Key == "" {
			return nil, fmt.Errorf("<default> requires a key attribute")
		}

		node, err := routeDefaultToNode(def)
		if err != nil {
			return nil, fmt.Errorf("default %q: %w", def.Key, err)
		}

		mapPut(mapping, def.Key, node)
	}

	return mapping, nil
}

func routeDefaultToNode(def xmlRouteDefault) (*yaml.Node, error) {
	if isXSINil(def.Nil) {
		return newNull("null"), nil
	}

	if len(def.Typed) > 1 {
		return nil, fmt.Errorf("only one typed value element is allowed")
	}

	if len(def.Typed) == 1 {
		return typedValueToNode(def.Typed[0])
	}

	// Symfony treats plain text content of a <default> as a string.
	return newString(strings.TrimSpace(def.Value)), nil
}

func typedValueToNode(value xmlTypedValue) (*yaml.Node, error) {
	if isXSINil(value.Nil) {
		return newNull("null"), nil
	}

	trimmed := strings.TrimSpace(value.Value)

	switch value.XMLName.Local {
	case "bool":
		return scalarValue(trimmed == "true" || trimmed == "1"), nil
	case "int":
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid <int> value %q", value.Value)
		}

		return scalarValue(parsed), nil
	case "float":
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid <float> value %q", value.Value)
		}

		return scalarValue(parsed), nil
	case "string":
		return newString(trimmed), nil
	case "list":
		sequence := newSequence()
		for _, child := range value.Children {
			node, err := typedValueToNode(child)
			if err != nil {
				return nil, err
			}

			sequence.Content = append(sequence.Content, node)
		}

		return sequence, nil
	case "map":
		mapping := newMapping()
		for _, child := range value.Children {
			if child.Key == "" {
				return nil, fmt.Errorf("<map> entries require a key attribute")
			}

			node, err := typedValueToNode(child)
			if err != nil {
				return nil, err
			}

			mapPut(mapping, child.Key, node)
		}

		return mapping, nil
	default:
		return nil, fmt.Errorf("unsupported typed value element <%s>", value.XMLName.Local)
	}
}
