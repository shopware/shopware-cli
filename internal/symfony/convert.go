package symfony

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConvertContainerToYAML converts a parsed services.xml container into the
// equivalent services.yaml content. It returns an error whenever the XML
// contains something that cannot be expressed safely in YAML, so a conversion
// never silently drops configuration.
func ConvertContainerToYAML(container *Container) ([]byte, error) {
	if err := unknownElementError("container", container.Unknown); err != nil {
		return nil, err
	}

	root := newMapping()

	if err := appendContainerSections(root, container.Imports, container.Parameters, container.Services); err != nil {
		return nil, err
	}

	for _, when := range container.When {
		if when.Env == "" {
			return nil, fmt.Errorf("<when> requires an env attribute")
		}

		if err := unknownElementError("when", when.Unknown); err != nil {
			return nil, err
		}

		whenMapping := newMapping()
		if err := appendContainerSections(whenMapping, when.Imports, when.Parameters, when.Services); err != nil {
			return nil, err
		}

		mapPut(root, "when@"+when.Env, whenMapping)
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

func appendContainerSections(target *yaml.Node, imports *xmlImports, parameters *xmlParameters, services *xmlServices) error {
	if imports != nil {
		node, err := importsToNode(imports)
		if err != nil {
			return err
		}

		if len(node.Content) > 0 {
			mapPut(target, "imports", node)
		}
	}

	if parameters != nil {
		node, err := parametersToNode(parameters)
		if err != nil {
			return err
		}

		if len(node.Content) > 0 {
			mapPut(target, "parameters", node)
		}
	}

	if services != nil {
		node, err := servicesToNode(services)
		if err != nil {
			return err
		}

		if len(node.Content) > 0 {
			mapPut(target, "services", node)
		}
	}

	return nil
}

func importsToNode(imports *xmlImports) (*yaml.Node, error) {
	if err := unknownElementError("imports", imports.Unknown); err != nil {
		return nil, err
	}

	sequence := newSequence()

	for _, imp := range imports.Imports {
		if err := unknownAttributeError("import", imp.UnknownAttrs); err != nil {
			return nil, err
		}

		if imp.Resource == "" {
			return nil, fmt.Errorf("<import> requires a resource attribute")
		}

		entry := newMapping()
		entry.Style = yaml.FlowStyle
		mapPut(entry, "resource", newString(imp.Resource))

		if imp.Type != "" {
			mapPut(entry, "type", newString(imp.Type))
		}

		if imp.IgnoreErrors != "" {
			mapPut(entry, "ignore_errors", scalarValue(phpize(imp.IgnoreErrors)))
		}

		sequence.Content = append(sequence.Content, entry)
	}

	return sequence, nil
}

func parametersToNode(parameters *xmlParameters) (*yaml.Node, error) {
	if err := unknownElementError("parameters", parameters.Unknown); err != nil {
		return nil, err
	}

	mapping := newMapping()

	for _, parameter := range parameters.Parameters {
		if parameter.Key == "" {
			return nil, fmt.Errorf("<parameter> requires a key attribute")
		}

		value, err := parameterValueToNode(parameter)
		if err != nil {
			return nil, fmt.Errorf("parameter %q: %w", parameter.Key, err)
		}

		mapPut(mapping, parameter.Key, value)
	}

	return mapping, nil
}

func parameterValueToNode(parameter xmlParameter) (*yaml.Node, error) {
	if err := unknownElementError("parameter", parameter.Unknown); err != nil {
		return nil, err
	}

	if err := unknownAttributeError("parameter", parameter.UnknownAttrs); err != nil {
		return nil, err
	}

	switch parameter.Type {
	case "collection":
		return parameterCollectionToNode(parameter.Children)
	case "constant":
		return taggedScalar("!php/const", strings.TrimSpace(parameter.Value)), nil
	case "binary":
		return taggedScalar("!!binary", strings.TrimSpace(parameter.Value)), nil
	case "string":
		return newString(parameter.Value), nil
	case "":
		if len(parameter.Children) > 0 {
			return parameterCollectionToNode(parameter.Children)
		}

		return scalarValue(phpize(parameter.Value)), nil
	default:
		return nil, fmt.Errorf("unsupported parameter type %q", parameter.Type)
	}
}

func parameterCollectionToNode(children []xmlParameter) (*yaml.Node, error) {
	keyed := false
	for _, child := range children {
		if child.Key != "" {
			keyed = true
			break
		}
	}

	if !keyed {
		sequence := newSequence()

		for _, child := range children {
			value, err := parameterValueToNode(child)
			if err != nil {
				return nil, err
			}

			sequence.Content = append(sequence.Content, value)
		}

		return sequence, nil
	}

	mapping := newMapping()

	for _, child := range children {
		if child.Key == "" {
			return nil, fmt.Errorf("collection mixes keyed and unkeyed <parameter> elements")
		}

		value, err := parameterValueToNode(child)
		if err != nil {
			return nil, err
		}

		mapPut(mapping, child.Key, value)
	}

	return mapping, nil
}

func servicesToNode(services *xmlServices) (*yaml.Node, error) {
	mapping := newMapping()

	if len(services.Defaults) > 1 {
		return nil, fmt.Errorf("multiple <defaults> elements are not supported")
	}

	if len(services.Defaults) == 1 {
		node, err := defaultsToNode(services.Defaults[0])
		if err != nil {
			return nil, err
		}

		if len(node.Content) > 0 {
			mapPut(mapping, "_defaults", node)
		}
	}

	instanceof := newMapping()
	seenInstanceof := map[string]bool{}

	for _, item := range services.Items {
		if item.Kind != "instanceof" {
			continue
		}

		if item.Service.ID == "" {
			return nil, fmt.Errorf("<instanceof> requires an id attribute")
		}

		if item.Service.Class != "" {
			return nil, fmt.Errorf("instanceof %q: the class attribute is not supported", item.Service.ID)
		}

		if err := rejectPrototypeFields(item.Service, fmt.Sprintf("instanceof %q", item.Service.ID)); err != nil {
			return nil, err
		}

		if seenInstanceof[item.Service.ID] {
			return nil, fmt.Errorf("duplicate <instanceof> for %q", item.Service.ID)
		}
		seenInstanceof[item.Service.ID] = true

		body, err := serviceBodyToNode(item.Service, false)
		if err != nil {
			return nil, fmt.Errorf("instanceof %q: %w", item.Service.ID, err)
		}

		mapPut(instanceof, item.Service.ID, body)
	}

	if len(instanceof.Content) > 0 {
		mapPut(mapping, "_instanceof", instanceof)
	}

	seen := map[string]bool{}

	for _, item := range services.Items {
		var (
			key  string
			node *yaml.Node
			err  error
		)

		switch item.Kind {
		case "service":
			key, node, err = serviceToNode(item.Service)
		case "prototype":
			key, node, err = prototypeToNode(item.Service)
		case "instanceof":
			continue
		default:
			return nil, fmt.Errorf("unsupported element <%s> inside <services>", item.Kind)
		}

		if err != nil {
			return nil, err
		}

		if seen[key] {
			return nil, fmt.Errorf("service %q is defined multiple times", key)
		}
		seen[key] = true

		mapPut(mapping, key, node)
	}

	return mapping, nil
}

func defaultsToNode(defaults xmlDefaults) (*yaml.Node, error) {
	if err := unknownElementError("defaults", defaults.Unknown); err != nil {
		return nil, err
	}

	if err := unknownAttributeError("defaults", defaults.UnknownAttrs); err != nil {
		return nil, err
	}

	mapping := newMapping()

	if defaults.Autowire != "" {
		mapPut(mapping, "autowire", scalarValue(phpize(defaults.Autowire)))
	}

	if defaults.Autoconfigure != "" {
		mapPut(mapping, "autoconfigure", scalarValue(phpize(defaults.Autoconfigure)))
	}

	if defaults.Public != "" {
		mapPut(mapping, "public", scalarValue(phpize(defaults.Public)))
	}

	if len(defaults.Binds) > 0 {
		node, err := bindsToNode(defaults.Binds)
		if err != nil {
			return nil, fmt.Errorf("defaults: %w", err)
		}

		mapPut(mapping, "bind", node)
	}

	if len(defaults.Tags) > 0 {
		node, err := tagsToNode(defaults.Tags)
		if err != nil {
			return nil, fmt.Errorf("defaults: %w", err)
		}

		mapPut(mapping, "tags", node)
	}

	return mapping, nil
}

func serviceToNode(service *xmlService) (string, *yaml.Node, error) {
	if service.ID == "" {
		return "", nil, fmt.Errorf("<service> requires an id attribute")
	}

	if err := rejectPrototypeFields(service, fmt.Sprintf("service %q", service.ID)); err != nil {
		return "", nil, err
	}

	if service.Alias != "" {
		node, err := aliasToNode(service)
		if err != nil {
			return "", nil, fmt.Errorf("alias %q: %w", service.ID, err)
		}

		return service.ID, node, nil
	}

	omitClass := service.Class == service.ID && strings.Contains(service.ID, "\\")

	body, err := serviceBodyToNode(service, omitClass)
	if err != nil {
		return "", nil, fmt.Errorf("service %q: %w", service.ID, err)
	}

	return service.ID, body, nil
}

func aliasToNode(service *xmlService) (*yaml.Node, error) {
	if err := unknownElementError("service", service.Unknown); err != nil {
		return nil, err
	}

	if err := unknownAttributeError("service", service.UnknownAttrs); err != nil {
		return nil, err
	}

	if hasDefinitionConfig(service) {
		return nil, fmt.Errorf("aliases only support the public attribute and <deprecated>")
	}

	if service.Public == "" && service.Deprecated == nil {
		return newString("@" + service.Alias), nil
	}

	mapping := newMapping()
	mapPut(mapping, "alias", newString(service.Alias))

	if service.Public != "" {
		mapPut(mapping, "public", scalarValue(phpize(service.Public)))
	}

	if service.Deprecated != nil {
		mapPut(mapping, "deprecated", deprecatedToNode(service.Deprecated))
	}

	return mapping, nil
}

func rejectPrototypeFields(service *xmlService, context string) error {
	if service.Namespace != "" || service.Resource != "" || service.ExcludeAttr != "" || len(service.Excludes) > 0 {
		return fmt.Errorf("%s: namespace, resource and exclude are only supported on <prototype>", context)
	}

	return nil
}

func hasDefinitionConfig(service *xmlService) bool {
	return service.Class != "" ||
		service.Shared != "" ||
		service.Synthetic != "" ||
		service.Lazy != "" ||
		service.Abstract != "" ||
		service.Autowire != "" ||
		service.Autoconfigure != "" ||
		service.Parent != "" ||
		service.Constructor != "" ||
		service.Decorates != "" ||
		service.DecorationInnerName != "" ||
		service.DecorationPriority != "" ||
		service.DecorationOnInvalid != "" ||
		service.File != "" ||
		len(service.Arguments) > 0 ||
		len(service.Properties) > 0 ||
		len(service.Calls) > 0 ||
		len(service.Tags) > 0 ||
		len(service.Binds) > 0 ||
		service.Factory != nil ||
		service.Configurator != nil
}

func prototypeToNode(prototype *xmlService) (string, *yaml.Node, error) {
	if prototype.Namespace == "" {
		return "", nil, fmt.Errorf("<prototype> requires a namespace attribute")
	}

	if prototype.Resource == "" {
		return "", nil, fmt.Errorf("prototype %q: requires a resource attribute", prototype.Namespace)
	}

	if prototype.ID != "" || prototype.Alias != "" || prototype.Class != "" {
		return "", nil, fmt.Errorf("prototype %q: id, alias and class are not supported", prototype.Namespace)
	}

	mapping := newMapping()
	mapPut(mapping, "resource", newString(prototype.Resource))

	if prototype.ExcludeAttr != "" && len(prototype.Excludes) > 0 {
		return "", nil, fmt.Errorf("prototype %q: mixes the exclude attribute with <exclude> elements", prototype.Namespace)
	}

	switch {
	case prototype.ExcludeAttr != "":
		mapPut(mapping, "exclude", newString(prototype.ExcludeAttr))
	case len(prototype.Excludes) == 1:
		mapPut(mapping, "exclude", newString(prototype.Excludes[0]))
	case len(prototype.Excludes) > 1:
		excludes := newSequence()
		for _, exclude := range prototype.Excludes {
			excludes.Content = append(excludes.Content, newString(exclude))
		}

		mapPut(mapping, "exclude", excludes)
	}

	body, err := serviceBodyToNode(prototype, false)
	if err != nil {
		return "", nil, fmt.Errorf("prototype %q: %w", prototype.Namespace, err)
	}

	if body.Kind == yaml.MappingNode {
		mapping.Content = append(mapping.Content, body.Content...)
	}

	return prototype.Namespace, mapping, nil
}

// serviceBodyToNode converts everything inside a service definition. It is
// shared between services, prototypes, instanceof conditionals and inline
// services. An empty definition is represented as null (`~`).
func serviceBodyToNode(service *xmlService, omitClass bool) (*yaml.Node, error) {
	if err := unknownElementError("service", service.Unknown); err != nil {
		return nil, err
	}

	if err := unknownAttributeError("service", service.UnknownAttrs); err != nil {
		return nil, err
	}

	mapping := newMapping()

	appendServiceAttributes(mapping, service, omitClass)

	if err := appendDecoration(mapping, service); err != nil {
		return nil, err
	}

	if err := appendServiceChildren(mapping, service); err != nil {
		return nil, err
	}

	if len(mapping.Content) == 0 {
		return newNull("~"), nil
	}

	return mapping, nil
}

func appendServiceAttributes(mapping *yaml.Node, service *xmlService, omitClass bool) {
	if service.Class != "" && !omitClass {
		mapPut(mapping, "class", newString(service.Class))
	}

	if service.File != "" {
		mapPut(mapping, "file", newString(service.File))
	}

	if service.Parent != "" {
		mapPut(mapping, "parent", newString(service.Parent))
	}

	phpizedAttributes := []struct {
		key   string
		value string
	}{
		{"lazy", service.Lazy},
		{"shared", service.Shared},
		{"synthetic", service.Synthetic},
		{"abstract", service.Abstract},
		{"public", service.Public},
		{"autowire", service.Autowire},
		{"autoconfigure", service.Autoconfigure},
	}

	for _, attribute := range phpizedAttributes {
		if attribute.value != "" {
			mapPut(mapping, attribute.key, scalarValue(phpize(attribute.value)))
		}
	}

	if service.Constructor != "" {
		mapPut(mapping, "constructor", newString(service.Constructor))
	}
}

func appendDecoration(mapping *yaml.Node, service *xmlService) error {
	if service.Decorates == "" {
		if service.DecorationInnerName != "" || service.DecorationPriority != "" || service.DecorationOnInvalid != "" {
			return fmt.Errorf("decoration attributes require the decorates attribute")
		}

		return nil
	}

	mapPut(mapping, "decorates", newString(service.Decorates))

	if service.DecorationInnerName != "" {
		mapPut(mapping, "decoration_inner_name", newString(service.DecorationInnerName))
	}

	if service.DecorationPriority != "" {
		mapPut(mapping, "decoration_priority", scalarValue(phpize(service.DecorationPriority)))
	}

	switch service.DecorationOnInvalid {
	case "":
	case "exception", "ignore":
		mapPut(mapping, "decoration_on_invalid", newString(service.DecorationOnInvalid))
	case "null":
		// XML uses the string "null" while YAML expects an actual null value.
		mapPut(mapping, "decoration_on_invalid", newNull("null"))
	default:
		return fmt.Errorf("unsupported decoration-on-invalid value %q", service.DecorationOnInvalid)
	}

	return nil
}

func appendServiceChildren(mapping *yaml.Node, service *xmlService) error {
	if service.Deprecated != nil {
		mapPut(mapping, "deprecated", deprecatedToNode(service.Deprecated))
	}

	if service.Factory != nil {
		node, err := callableToNode(service.Factory, "factory")
		if err != nil {
			return err
		}

		mapPut(mapping, "factory", node)
	}

	if service.Configurator != nil {
		node, err := callableToNode(service.Configurator, "configurator")
		if err != nil {
			return err
		}

		mapPut(mapping, "configurator", node)
	}

	if len(service.Arguments) > 0 {
		node, err := argumentsToNode(service.Arguments, service.Parent != "")
		if err != nil {
			return err
		}

		mapPut(mapping, "arguments", node)
	}

	if len(service.Properties) > 0 {
		node, err := propertiesToNode(service.Properties)
		if err != nil {
			return err
		}

		mapPut(mapping, "properties", node)
	}

	if len(service.Binds) > 0 {
		node, err := bindsToNode(service.Binds)
		if err != nil {
			return err
		}

		mapPut(mapping, "bind", node)
	}

	if len(service.Calls) > 0 {
		node, err := callsToNode(service.Calls)
		if err != nil {
			return err
		}

		mapPut(mapping, "calls", node)
	}

	if len(service.Tags) > 0 {
		node, err := tagsToNode(service.Tags)
		if err != nil {
			return err
		}

		mapPut(mapping, "tags", node)
	}

	return nil
}

func deprecatedToNode(deprecated *xmlDeprecated) *yaml.Node {
	mapping := newMapping()

	if deprecated.Package != "" {
		mapPut(mapping, "package", newString(deprecated.Package))
	}

	if deprecated.Version != "" {
		mapPut(mapping, "version", newString(deprecated.Version))
	}

	if message := strings.TrimSpace(deprecated.Message); message != "" {
		mapPut(mapping, "message", newString(message))
	}

	return mapping
}

func callableToNode(callable *xmlCallable, element string) (*yaml.Node, error) {
	if err := unknownElementError(element, callable.Unknown); err != nil {
		return nil, err
	}

	if err := unknownAttributeError(element, callable.UnknownAttrs); err != nil {
		return nil, err
	}

	if callable.Expression != "" {
		return newString("@=" + callable.Expression), nil
	}

	if callable.Function != "" {
		return newString(callable.Function), nil
	}

	var target *yaml.Node

	switch {
	case callable.Service != "":
		target = newString("@" + callable.Service)
	case callable.Class != "":
		target = newString(callable.Class)
	case element == "factory" && callable.Method != "":
		// A factory with only a method refers to the service class itself.
		target = newNull("null")
	default:
		return nil, fmt.Errorf("<%s> requires a class, service, function or expression attribute", element)
	}

	method := callable.Method
	if method == "" {
		method = "__invoke"
	}

	pair := newSequence()
	pair.Style = yaml.FlowStyle
	pair.Content = append(pair.Content, target, newString(method))

	return pair, nil
}

func argumentsToNode(arguments []xmlArgument, isChildDefinition bool) (*yaml.Node, error) {
	keyed := false
	for _, argument := range arguments {
		if argument.Key != "" || argument.Index != "" {
			keyed = true
			break
		}
	}

	if !keyed {
		sequence := newSequence()

		for _, argument := range arguments {
			node, err := argumentValueToNode(argument)
			if err != nil {
				return nil, err
			}

			sequence.Content = append(sequence.Content, node)
		}

		return sequence, nil
	}

	mapping := newMapping()
	nextIndex := int64(0)

	for _, argument := range arguments {
		var key *yaml.Node

		switch {
		case argument.Key != "":
			key = newString(argument.Key)
		case argument.Index != "":
			index, ok := phpize(argument.Index).(int64)
			if !ok {
				return nil, fmt.Errorf("invalid argument index %q", argument.Index)
			}

			if isChildDefinition {
				key = newString(fmt.Sprintf("index_%d", index))
			} else {
				key = scalarValue(index)
			}

			if index >= nextIndex {
				nextIndex = index + 1
			}
		default:
			key = scalarValue(nextIndex)
			nextIndex++
		}

		node, err := argumentValueToNode(argument)
		if err != nil {
			return nil, err
		}

		mapping.Content = append(mapping.Content, key, node)
	}

	return mapping, nil
}

func argumentValueToNode(argument xmlArgument) (*yaml.Node, error) {
	if err := unknownElementError("argument", argument.Unknown); err != nil {
		return nil, err
	}

	if err := unknownAttributeError("argument", argument.UnknownAttrs); err != nil {
		return nil, err
	}

	if len(argument.InlineServices) > 0 && argument.Type != "service" {
		return nil, fmt.Errorf("inline <service> elements require type=\"service\"")
	}

	if len(argument.Excludes) > 0 && argument.Type != "tagged" && argument.Type != "tagged_iterator" && argument.Type != "tagged_locator" {
		return nil, fmt.Errorf("<exclude> elements are only supported on tagged iterator arguments")
	}

	switch argument.Type {
	case "":
		if len(argument.Children) > 0 {
			return argumentsToNode(argument.Children, false)
		}

		return phpizedServiceValue(argument.Value), nil
	case "collection":
		return argumentsToNode(argument.Children, false)
	case "service":
		if len(argument.InlineServices) > 1 {
			return nil, fmt.Errorf("only one inline <service> is supported per argument")
		}

		if len(argument.InlineServices) == 1 {
			return inlineServiceToNode(argument.InlineServices[0])
		}

		if argument.ID == "" {
			return nil, fmt.Errorf("argument type=\"service\" requires an id attribute")
		}

		return referenceNode(argument.ID, argument.OnInvalid)
	case "expression":
		return newString("@=" + argument.Value), nil
	case "string":
		return serviceString(argument.Value), nil
	case "constant":
		return taggedScalar("!php/const", strings.TrimSpace(argument.Value)), nil
	case "binary":
		return taggedScalar("!!binary", strings.TrimSpace(argument.Value)), nil
	case "abstract":
		return taggedScalar("!abstract", argument.Value), nil
	case "iterator":
		node, err := argumentsToNode(argument.Children, false)
		if err != nil {
			return nil, err
		}

		node.Tag = "!iterator"

		return node, nil
	case "service_locator":
		node, err := argumentsToNode(argument.Children, false)
		if err != nil {
			return nil, err
		}

		node.Tag = "!service_locator"

		return node, nil
	case "service_closure", "closure":
		if argument.ID == "" {
			return nil, fmt.Errorf("argument type=%q requires an id attribute", argument.Type)
		}

		node, err := referenceNode(argument.ID, argument.OnInvalid)
		if err != nil {
			return nil, err
		}

		node.Tag = "!" + argument.Type

		return node, nil
	case "tagged", "tagged_iterator", "tagged_locator":
		return taggedIteratorToNode(argument)
	default:
		return nil, fmt.Errorf("unsupported argument type %q", argument.Type)
	}
}

func taggedIteratorToNode(argument xmlArgument) (*yaml.Node, error) {
	if argument.Tag == "" {
		return nil, fmt.Errorf("argument type=%q requires a tag attribute", argument.Type)
	}

	yamlTag := "!tagged_iterator"
	if argument.Type == "tagged_locator" {
		yamlTag = "!tagged_locator"
	}

	if argument.IndexBy == "" && argument.DefaultIndexMethod == "" && argument.DefaultPriorityMethod == "" && argument.ExcludeSelf == "" && len(argument.Excludes) == 0 {
		return taggedScalar(yamlTag, argument.Tag), nil
	}

	mapping := newMapping()
	mapping.Style = yaml.FlowStyle
	mapping.Tag = yamlTag
	mapPut(mapping, "tag", newString(argument.Tag))

	if argument.IndexBy != "" {
		mapPut(mapping, "index_by", newString(argument.IndexBy))
	}

	if argument.DefaultIndexMethod != "" {
		mapPut(mapping, "default_index_method", newString(argument.DefaultIndexMethod))
	}

	if argument.DefaultPriorityMethod != "" {
		mapPut(mapping, "default_priority_method", newString(argument.DefaultPriorityMethod))
	}

	if len(argument.Excludes) > 0 {
		excludes := newSequence()
		excludes.Style = yaml.FlowStyle
		for _, exclude := range argument.Excludes {
			excludes.Content = append(excludes.Content, newString(exclude))
		}

		mapPut(mapping, "exclude", excludes)
	}

	if argument.ExcludeSelf != "" {
		mapPut(mapping, "exclude_self", scalarValue(phpize(argument.ExcludeSelf)))
	}

	return mapping, nil
}

func inlineServiceToNode(service xmlService) (*yaml.Node, error) {
	if service.ID != "" || service.Alias != "" {
		return nil, fmt.Errorf("inline services do not support id or alias attributes")
	}

	if service.Class == "" {
		return nil, fmt.Errorf("inline services require a class attribute")
	}

	if err := rejectPrototypeFields(&service, "inline service"); err != nil {
		return nil, err
	}

	node, err := serviceBodyToNode(&service, false)
	if err != nil {
		return nil, err
	}

	node.Tag = "!service"

	return node, nil
}

func referenceNode(id string, onInvalid string) (*yaml.Node, error) {
	prefix := "@"

	switch onInvalid {
	case "", "exception":
	case "ignore":
		prefix = "@?"
	case "null":
		// "@?" means ignore, which removes method calls and collection
		// entries instead of passing null for them, so the null strategy
		// cannot be expressed in YAML.
		return nil, fmt.Errorf(`on-invalid="null" is not supported by the YAML format, change it to on-invalid="ignore" first if dropping the dependency is acceptable`)
	case "ignore_uninitialized":
		prefix = "@!"
	default:
		return nil, fmt.Errorf("unsupported on-invalid value %q", onInvalid)
	}

	return newString(prefix + id), nil
}

func propertiesToNode(properties []xmlProperty) (*yaml.Node, error) {
	mapping := newMapping()

	for _, property := range properties {
		if property.Name == "" {
			return nil, fmt.Errorf("<property> requires a name attribute")
		}

		node, err := argumentValueToNode(property.xmlArgument)
		if err != nil {
			return nil, fmt.Errorf("property %q: %w", property.Name, err)
		}

		mapPut(mapping, property.Name, node)
	}

	return mapping, nil
}

func bindsToNode(binds []xmlArgument) (*yaml.Node, error) {
	mapping := newMapping()

	for _, bind := range binds {
		if bind.Key == "" {
			return nil, fmt.Errorf("<bind> requires a key attribute")
		}

		key := bind.Key
		bind.Key = ""

		node, err := argumentValueToNode(bind)
		if err != nil {
			return nil, fmt.Errorf("bind %q: %w", key, err)
		}

		mapPut(mapping, key, node)
	}

	return mapping, nil
}

func callsToNode(calls []xmlCall) (*yaml.Node, error) {
	sequence := newSequence()

	for _, call := range calls {
		if err := unknownElementError("call", call.Unknown); err != nil {
			return nil, err
		}

		if err := unknownAttributeError("call", call.UnknownAttrs); err != nil {
			return nil, err
		}

		if call.Method == "" {
			return nil, fmt.Errorf("<call> requires a method attribute")
		}

		entry := newSequence()
		entry.Style = yaml.FlowStyle
		entry.Content = append(entry.Content, newString(call.Method))

		returnsClone := phpize(call.ReturnsClone) == true

		if len(call.Arguments) > 0 || returnsClone {
			arguments, err := argumentsToNode(call.Arguments, false)
			if err != nil {
				return nil, fmt.Errorf("call %q: %w", call.Method, err)
			}

			entry.Content = append(entry.Content, arguments)
		}

		if returnsClone {
			entry.Content = append(entry.Content, scalarValue(true))
		}

		sequence.Content = append(sequence.Content, entry)
	}

	return sequence, nil
}

func tagsToNode(tags []xmlTag) (*yaml.Node, error) {
	sequence := newSequence()

	for _, tag := range tags {
		node, err := tagToNode(tag)
		if err != nil {
			return nil, err
		}

		sequence.Content = append(sequence.Content, node)
	}

	return sequence, nil
}

func tagToNode(tag xmlTag) (*yaml.Node, error) {
	name := tag.NameAttr
	if name == "" {
		name = strings.TrimSpace(tag.Value)
	}

	if name == "" {
		return nil, fmt.Errorf("<tag> requires a name")
	}

	if len(tag.OtherAttrs) == 0 && len(tag.Attributes) == 0 {
		return newString(name), nil
	}

	mapping := newMapping()
	mapping.Style = yaml.FlowStyle
	mapPut(mapping, "name", newString(name))

	for _, attr := range tag.OtherAttrs {
		putTagAttribute(mapping, attr.Name.Local, scalarValue(phpize(attr.Value)))
	}

	for _, attribute := range tag.Attributes {
		node, err := tagAttributeToNode(attribute)
		if err != nil {
			return nil, err
		}

		putTagAttribute(mapping, attribute.Name, node)
	}

	return mapping, nil
}

func tagAttributeToNode(attribute xmlTagAttribute) (*yaml.Node, error) {
	if attribute.Name == "" {
		return nil, fmt.Errorf("tag <attribute> requires a name attribute")
	}

	if len(attribute.Children) == 0 {
		return scalarValue(phpize(attribute.Value)), nil
	}

	mapping := newMapping()
	mapping.Style = yaml.FlowStyle

	for _, child := range attribute.Children {
		node, err := tagAttributeToNode(child)
		if err != nil {
			return nil, err
		}

		putTagAttribute(mapping, child.Name, node)
	}

	return mapping, nil
}

// putTagAttribute mirrors Symfony's XmlFileLoader, which stores tag attributes
// containing dashes additionally under their underscore variant.
func putTagAttribute(mapping *yaml.Node, name string, value *yaml.Node) {
	if strings.Contains(name, "-") && !strings.Contains(name, "_") {
		normalized := strings.ReplaceAll(name, "-", "_")

		if !mappingHasKey(mapping, normalized) {
			mapPut(mapping, normalized, value)
		}
	}

	mapPut(mapping, name, value)
}

func mappingHasKey(mapping *yaml.Node, key string) bool {
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return true
		}
	}

	return false
}

func phpizedServiceValue(value string) *yaml.Node {
	if phpized, ok := phpize(value).(string); ok {
		return serviceString(phpized)
	}

	return scalarValue(phpize(value))
}

// serviceString creates a string node for values used inside service
// definitions. Literal "@" prefixes are escaped as "@@", since "@" marks
// service references in YAML files (unlike in XML text content).
func serviceString(value string) *yaml.Node {
	if strings.HasPrefix(value, "@") {
		value = "@" + value
	}

	return newString(value)
}

func newMapping() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

func newSequence() *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
}

func newString(value string) *yaml.Node {
	node := &yaml.Node{}
	node.SetString(value)

	return node
}

func newNull(representation string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: representation}
}

func taggedScalar(tag string, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
}

func scalarValue(value any) *yaml.Node {
	if value == nil {
		return newNull("null")
	}

	if str, ok := value.(string); ok {
		return newString(str)
	}

	node := &yaml.Node{}
	if err := node.Encode(value); err != nil {
		return newString(fmt.Sprintf("%v", value))
	}

	return node
}

func mapPut(mapping *yaml.Node, key string, value *yaml.Node) {
	mapping.Content = append(mapping.Content, newString(key), value)
}

// insertBlankLines adds an empty line between top-level sections and between
// the service definitions to keep the generated YAML readable.
func insertBlankLines(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	out := make([]string, 0, len(lines))

	inServices := false
	firstService := true

	for _, line := range lines {
		if len(line) > 0 && line[0] != ' ' && line[0] != '-' {
			if len(out) > 0 {
				out = append(out, "")
			}

			inServices = line == "services:"
			firstService = true
			out = append(out, line)

			continue
		}

		if inServices && isServiceKeyLine(line) {
			if firstService {
				firstService = false
			} else {
				out = append(out, "")
			}
		}

		out = append(out, line)
	}

	return []byte(strings.Join(out, "\n"))
}

func isServiceKeyLine(line string) bool {
	return len(line) > 4 && strings.HasPrefix(line, "    ") && line[4] != ' '
}
