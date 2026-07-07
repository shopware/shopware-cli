package proxy

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// UpdateProjectConfigURL sets the url key in the project config
// (.shopware-project.yml), creating the file when it does not exist. Existing
// configuration, ordering and comments are preserved.
func UpdateProjectConfigURL(configPath, url string) error {
	content, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}

		return os.WriteFile(configPath, fmt.Appendf(nil, "url: %s\n", url), 0o600)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", configPath, err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return os.WriteFile(configPath, fmt.Appendf(nil, "url: %s\n", url), 0o600)
	}

	mapping := doc.Content[0]

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "url" {
			value := mapping.Content[i+1]
			value.SetString(url)

			return writeYamlNode(configPath, &doc)
		}
	}

	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "url"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: url},
	)

	return writeYamlNode(configPath, &doc)
}

func writeYamlNode(path string, node *yaml.Node) error {
	out, err := yaml.Marshal(node)
	if err != nil {
		return err
	}

	return os.WriteFile(path, out, 0o600)
}
