package shop

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/system"
)

// The helpers below add, change and remove environments.<name> blocks of the
// project config in place (via yaml.Node), for the same reason as the url
// patching: comments, key order, ${VAR} references and unrelated settings of
// a committed file must survive a change to a single environment.

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

// ValidateEnvironmentName accepts letters, numbers and dashes.
func ValidateEnvironmentName(name string) error {
	if !environmentNamePattern.MatchString(name) {
		return errors.New("use letters, numbers and dashes, starting with a letter or number")
	}
	return nil
}

// EnvironmentChange describes an edit of one environment. Nil fields are left
// alone; an empty string or a zero port removes the key so the default applies.
type EnvironmentChange struct {
	// Name renames the environment.
	Name *string
	URL  *string

	Host         *string
	User         *string
	Port         *int
	Directory    *string
	IdentityFile *string
	PHPBinary    *string
}

// Empty reports whether the change leaves the environment untouched.
func (c EnvironmentChange) Empty() bool {
	return c.Name == nil && c.URL == nil && !c.changesSSH()
}

func (c EnvironmentChange) changesSSH() bool {
	return c.Host != nil || c.User != nil || c.Port != nil || c.Directory != nil || c.IdentityFile != nil || c.PHPBinary != nil
}

// AddEnvironment appends environments.<name> to the project config file.
func AddEnvironment(configPath, name string, env *EnvironmentConfig) error {
	if err := ValidateEnvironmentName(name); err != nil {
		return fmt.Errorf("cannot add environment %q: %w", name, err)
	}
	if err := validateEnvironment(env); err != nil {
		return fmt.Errorf("cannot add environment %q: %w", name, err)
	}

	doc, root, err := loadEnvironmentConfigDoc(configPath)
	if err != nil {
		return err
	}

	environments := configMapValue(root, "environments")
	if environments == nil {
		environments = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, scalarNode("environments"), environments)
	}
	if environments.Kind != yaml.MappingNode {
		return fmt.Errorf("cannot add environment %q: environments in %s is not a mapping", name, configPath)
	}
	if configMapValue(environments, name) != nil {
		return fmt.Errorf("cannot add environment %q: it already exists in %s", name, configPath)
	}

	var envNode yaml.Node
	if err := envNode.Encode(env); err != nil {
		return fmt.Errorf("cannot add environment %q: %w", name, err)
	}
	environments.Content = append(environments.Content, scalarNode(name), &envNode)

	return writeConfigDoc(configPath, doc)
}

// UpdateEnvironment applies change to environments.<name> of the project config
// file. The resulting environment is validated before anything is written.
func UpdateEnvironment(configPath, name string, change EnvironmentChange) error {
	doc, root, err := loadEnvironmentConfigDoc(configPath)
	if err != nil {
		return err
	}

	environments := configMapValue(root, "environments")
	env := ownedEnvironmentNode(environments, name)
	if env == nil {
		return fmt.Errorf("cannot edit environment %q: it is not defined in %s", name, configPath)
	}

	if change.URL != nil {
		setConfigScalar(env, "url", *change.URL, "!!str")
	}

	if change.changesSSH() {
		ssh := configMapValue(env, "ssh")
		if ssh == nil {
			ssh = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			env.Content = append(env.Content, scalarNode("ssh"), ssh)
		}
		if ssh.Kind != yaml.MappingNode {
			return fmt.Errorf("cannot edit environment %q: environments.%s.ssh in %s is not a mapping", name, name, configPath)
		}

		var port *string
		if change.Port != nil {
			port = new("")
			if *change.Port != 0 {
				*port = strconv.Itoa(*change.Port)
			}
		}
		// A slice keeps the order of newly appended keys stable.
		for _, field := range []struct {
			key, tag string
			value    *string
		}{
			{"host", "!!str", change.Host},
			{"user", "!!str", change.User},
			{"port", "!!int", port},
			{"directory", "!!str", change.Directory},
			{"identity_file", "!!str", change.IdentityFile},
			{"php_binary", "!!str", change.PHPBinary},
		} {
			if field.value != nil {
				setConfigScalar(ssh, field.key, *field.value, field.tag)
			}
		}
	}

	if err := validateEnvironmentEdit(doc, name); err != nil {
		return fmt.Errorf("cannot edit environment %q: %w", name, err)
	}

	if change.Name != nil && *change.Name != name {
		if err := ValidateEnvironmentName(*change.Name); err != nil {
			return fmt.Errorf("cannot rename environment %q to %q: %w", name, *change.Name, err)
		}
		if configMapValue(environments, *change.Name) != nil {
			return fmt.Errorf("cannot rename environment %q to %q: it already exists in %s", name, *change.Name, configPath)
		}
		for i := 0; i+1 < len(environments.Content); i += 2 {
			if environments.Content[i].Value == name {
				environments.Content[i].SetString(*change.Name)
			}
		}
	}

	return writeConfigDoc(configPath, doc)
}

// RemoveEnvironment deletes environments.<name> from the project config file.
func RemoveEnvironment(configPath, name string) error {
	doc, root, err := loadEnvironmentConfigDoc(configPath)
	if err != nil {
		return err
	}

	environments := configMapValue(root, "environments")
	if ownedEnvironmentNode(environments, name) == nil {
		return fmt.Errorf("cannot remove environment %q: it is not defined in %s", name, configPath)
	}
	removeConfigMapKey(environments, name)
	if err := checkEnvironmentAliases(doc); err != nil {
		return fmt.Errorf("cannot remove environment %q: %w", name, err)
	}

	return writeConfigDoc(configPath, doc)
}

func validateEnvironmentEdit(doc *yaml.Node, name string) error {
	if err := checkEnvironmentAliases(doc); err != nil {
		return err
	}

	// Expand variables the same way as ReadConfig, using the whole document
	// so the edited environment can reference anchors elsewhere in the file.
	content, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	var resolved yaml.Node
	if err := yaml.Unmarshal([]byte(system.ExpandEnv(string(content))), &resolved); err != nil {
		return err
	}
	if len(resolved.Content) == 0 {
		return errors.New("expanded project config is empty")
	}
	candidate := ownedEnvironmentNode(configMapValue(resolved.Content[0], "environments"), name)
	if candidate == nil {
		return errors.New("expanded project config has no matching environment")
	}
	var updated EnvironmentConfig
	if err := candidate.Decode(&updated); err != nil {
		return err
	}
	return validateEnvironment(&updated)
}

func checkEnvironmentAliases(doc *yaml.Node) error {
	// Follow Content only. An Alias can still point to an anchor removed from
	// the document, or to an earlier occurrence of a reused anchor name.
	anchors := make(map[*yaml.Node]struct{})
	var aliases []*yaml.Node
	var visit func(*yaml.Node)
	visit = func(node *yaml.Node) {
		if node.Anchor != "" {
			anchors[node] = struct{}{}
		}
		if node.Kind == yaml.AliasNode {
			aliases = append(aliases, node)
		}
		for _, child := range node.Content {
			visit(child)
		}
	}
	visit(doc)
	for _, alias := range aliases {
		if _, ok := anchors[alias.Alias]; !ok {
			return fmt.Errorf("YAML anchor %q is still referenced", alias.Value)
		}
	}
	return nil
}

func loadEnvironmentConfigDoc(configPath string) (*yaml.Node, *yaml.Node, error) {
	doc, root, err := loadConfigDoc(configPath)
	if os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("cannot find project configuration file %q, use shopware-cli project config init to create one", configPath)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read project config %s: %w", configPath, err)
	}
	return doc, root, nil
}

// ownedEnvironmentNode returns the mapping of an environment defined in this
// file. Environments that only exist in an include or the local override
// cannot be edited through the main config file.
func ownedEnvironmentNode(environments *yaml.Node, name string) *yaml.Node {
	if environments == nil || environments.Kind != yaml.MappingNode {
		return nil
	}
	env := configMapValue(environments, name)
	if env == nil || env.Kind != yaml.MappingNode {
		return nil
	}
	return env
}

func validateEnvironment(env *EnvironmentConfig) error {
	if env == nil {
		return errors.New("it has no configuration")
	}
	if env.Type != "ssh" {
		return nil
	}
	if env.SSH == nil {
		return errors.New("ssh environments need ssh.host and ssh.directory")
	}
	return env.SSH.Validate()
}

func scalarNode(value string) *yaml.Node {
	node := &yaml.Node{}
	node.SetString(value)
	return node
}

// setConfigScalar sets key in a mapping node, appending it when missing. An
// empty value removes the key.
func setConfigScalar(mapping *yaml.Node, key, value, tag string) {
	if value == "" {
		removeConfigMapKey(mapping, key)
		return
	}

	node := configMapValue(mapping, key)
	if node == nil {
		node = &yaml.Node{}
		mapping.Content = append(mapping.Content, scalarNode(key), node)
	}
	node.SetString(value)
	if tag != "!!str" {
		node.Kind, node.Tag, node.Style = yaml.ScalarNode, tag, 0
	}
}
