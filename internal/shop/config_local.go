package shop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"
)

// LocalConfigFileName returns the path of the local override file belonging to
// the given project configuration file.
func LocalConfigFileName(configPath string) string {
	return localConfigFileName(configPath)
}

// updateLocalConfig loads the local override file belonging to configPath
// (creating an empty document when it does not exist yet), lets mutate edit the
// root mapping node and writes the result back with 0600 permissions. The file
// is read raw — without environment substitution — so ${VAR} references,
// comments and !override/!reset tags survive the round-trip.
func updateLocalConfig(configPath string, mutate func(root *yaml.Node) error) error {
	localFile := localConfigFileName(configPath)

	var doc yaml.Node
	data, err := os.ReadFile(localFile)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("reading local config %s: %w", localFile, err)
	default:
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parsing local config %s: %w", localFile, err)
		}
	}

	root, err := documentRoot(&doc)
	if err != nil {
		return fmt.Errorf("updating local config %s: %w", localFile, err)
	}

	if err := mutate(root); err != nil {
		return fmt.Errorf("updating local config %s: %w", localFile, err)
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshalling local config %s: %w", localFile, err)
	}

	// The local override holds profiler secrets, so the content must never be
	// visible under a pre-existing permissive mode. Create the temporary file
	// with 0600 (and chmod it, because umask can strip bits) before the rename
	// publishes it atomically.
	tmp, err := os.CreateTemp(filepath.Dir(localFile), ".shopware-local-*.yml")
	if err != nil {
		return fmt.Errorf("writing local config %s: %w", localFile, err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("setting permissions on local config %s: %w", localFile, err)
	}
	if _, err := tmp.Write(out); err != nil {
		return fmt.Errorf("writing local config %s: %w", localFile, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing local config %s: %w", localFile, err)
	}
	if err := os.Rename(tmpName, localFile); err != nil {
		return fmt.Errorf("writing local config %s: %w", localFile, err)
	}

	return nil
}

// UpdateLocalDockerPorts merges the given host-port overrides into docker.ports
// of the local override file, preserving all other content.
func UpdateLocalDockerPorts(configPath string, ports map[string]int) error {
	if len(ports) == 0 {
		return nil
	}

	return updateLocalConfig(configPath, func(root *yaml.Node) error {
		docker, err := findOrCreateMapping(root, "docker")
		if err != nil {
			return err
		}

		portsNode, err := findOrCreateMapping(docker, "ports")
		if err != nil {
			return err
		}

		keys := make([]string, 0, len(ports))
		for key := range ports {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			setMappingValue(portsNode, key, &yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!int",
				Value: strconv.Itoa(ports[key]),
			})
		}

		return nil
	})
}

// UpdateLocalDockerPHP writes the profiler credential fields into docker.php
// of the local override file, preserving all other content. Empty fields
// remove the corresponding key so rotated or disabled credentials do not
// survive on disk; a nil php config clears all known credential keys.
func UpdateLocalDockerPHP(configPath string, php *ConfigDockerPHP) error {
	type credential struct {
		key   string
		value string
	}
	credentials := []credential{
		{key: "blackfire_server_id"},
		{key: "blackfire_server_token"},
		{key: "tideways_api_key"},
	}
	if php != nil {
		credentials[0].value = php.BlackfireServerID
		credentials[1].value = php.BlackfireServerToken
		credentials[2].value = php.TidewaysAPIKey
	}

	return updateLocalConfig(configPath, func(root *yaml.Node) error {
		docker, err := findOrCreateMapping(root, "docker")
		if err != nil {
			return err
		}

		phpNode, err := findOrCreateMapping(docker, "php")
		if err != nil {
			return err
		}

		for _, credential := range credentials {
			if credential.value == "" {
				deleteMappingKey(phpNode, credential.key)
				continue
			}

			setMappingValue(phpNode, credential.key, &yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!str",
				Value: credential.value,
			})
		}

		return nil
	})
}

// documentRoot returns the root mapping node of the document, creating it for
// empty or missing documents.
func documentRoot(doc *yaml.Node) (*yaml.Node, error) {
	if doc.Kind == 0 || len(doc.Content) == 0 {
		root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		doc.Kind = yaml.DocumentNode
		doc.Tag = ""
		doc.Content = []*yaml.Node{root}
		return root, nil
	}

	root := doc.Content[0]
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		root.Kind = yaml.MappingNode
		root.Tag = "!!map"
		root.Value = ""
		return root, nil
	}

	if root.Kind != yaml.MappingNode {
		return nil, errors.New("expected a YAML mapping at the document root")
	}

	return root, nil
}

// findOrCreateMapping returns the mapping node stored under key, appending a
// new empty mapping when the key does not exist. A null value (e.g. "docker:")
// is converted into a mapping in place.
func findOrCreateMapping(parent *yaml.Node, key string) (*yaml.Node, error) {
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value != key {
			continue
		}

		child := parent.Content[i+1]
		if child.Kind == yaml.ScalarNode && child.Tag == "!!null" {
			child.Kind = yaml.MappingNode
			child.Tag = "!!map"
			child.Value = ""
			return child, nil
		}

		if child.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("expected %q to be a YAML mapping", key)
		}

		return child, nil
	}

	child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		child,
	)

	return child, nil
}

// setMappingValue sets key to value in the mapping node, replacing an existing
// entry or appending a new one.
func setMappingValue(parent *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			parent.Content[i+1] = value
			return
		}
	}

	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}

// deleteMappingKey removes key from the mapping node if present.
func deleteMappingKey(parent *yaml.Node, key string) {
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			parent.Content = append(parent.Content[:i], parent.Content[i+2:]...)
			return
		}
	}
}
