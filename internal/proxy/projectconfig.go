package proxy

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ConfigURLState captures the url values of a project config file
// (.shopware-project.yml) before proxy registration, so deregistration can
// restore them exactly. The rest of the CLI (dev TUI, admin API client)
// resolves the shop URL from these keys, which is why registration points
// them at the proxy hostname.
type ConfigURLState struct {
	// HasFile is false when the project has no config file; registration
	// then leaves the project config alone entirely.
	HasFile bool `json:"-"`
	// RootURL is the top-level url value. HasRoot false = key was absent.
	RootURL string `json:"root_url,omitempty"`
	HasRoot bool   `json:"had_root_url,omitempty"`
	// EnvURL is environments.<env>.url, which overrides the top-level url
	// in the dev TUI and the admin API client. HasEnv false = key absent.
	EnvURL string `json:"env_url,omitempty"`
	HasEnv bool   `json:"had_env_url,omitempty"`
}

// envKey resolves the environments map key the CLI would use: an explicit
// environment name, or "local" (mirroring Config.ResolveEnvironment).
func envKey(envName string) string {
	if envName == "" {
		return "local"
	}

	return envName
}

// ReadProjectConfigURLs reads the current url values from the project config.
// A missing file is not an error; it yields HasFile=false.
func ReadProjectConfigURLs(configPath, envName string) (ConfigURLState, error) {
	_, root, err := loadConfigDoc(configPath)
	if os.IsNotExist(err) {
		return ConfigURLState{}, nil
	}
	if err != nil {
		return ConfigURLState{}, err
	}

	state := ConfigURLState{HasFile: true}

	if node := mapValue(root, "url"); node != nil {
		state.RootURL, state.HasRoot = node.Value, true
	}

	if envURL := envURLNode(root, envName); envURL != nil {
		state.EnvURL, state.HasEnv = envURL.Value, true
	}

	return state, nil
}

// SetProjectConfigURLs points the project config at url: the top-level url
// key always (created when missing), the environment url only when it
// already exists — an absent environment url already falls back to the
// top-level one, so there is nothing to override. Comments, ordering and
// unknown keys are preserved.
func SetProjectConfigURLs(configPath, envName, url string) error {
	doc, root, err := loadConfigDoc(configPath)
	if err != nil {
		return err
	}

	setMapValue(root, "url", url)

	if envURL := envURLNode(root, envName); envURL != nil {
		envURL.SetString(url)
	}

	return writeConfigDoc(configPath, doc)
}

// RestoreProjectConfigURLs puts the url values captured in prev back:
// previously present keys get their old value, a previously absent top-level
// url is removed again. An environment url we never touched stays untouched.
func RestoreProjectConfigURLs(configPath, envName string, prev ConfigURLState) error {
	doc, root, err := loadConfigDoc(configPath)
	if err != nil {
		return err
	}

	if prev.HasRoot {
		setMapValue(root, "url", prev.RootURL)
	} else {
		removeMapKey(root, "url")
	}

	if prev.HasEnv {
		if envURL := envURLNode(root, envName); envURL != nil {
			envURL.SetString(prev.EnvURL)
		}
	}

	return writeConfigDoc(configPath, doc)
}

// envURLNode returns the environments.<env>.url value node, or nil.
func envURLNode(root *yaml.Node, envName string) *yaml.Node {
	environments := mapValue(root, "environments")
	if environments == nil || environments.Kind != yaml.MappingNode {
		return nil
	}

	env := mapValue(environments, envKey(envName))
	if env == nil || env.Kind != yaml.MappingNode {
		return nil
	}

	return mapValue(env, "url")
}

func loadConfigDoc(path string) (*yaml.Node, *yaml.Node, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("%s does not contain a YAML mapping", path)
	}

	return &doc, doc.Content[0], nil
}

func writeConfigDoc(path string, doc *yaml.Node) error {
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}

	return os.WriteFile(path, out, 0o644)
}

// mapValue returns the value node for key in a mapping node, or nil.
func mapValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}

	return nil
}

// setMapValue updates key's value in a mapping node, appending the pair when
// the key is missing.
func setMapValue(mapping *yaml.Node, key, value string) {
	if node := mapValue(mapping, key); node != nil {
		node.SetString(value)
		return
	}

	keyNode := &yaml.Node{}
	keyNode.SetString(key)
	valueNode := &yaml.Node{}
	valueNode.SetString(value)

	mapping.Content = append(mapping.Content, keyNode, valueNode)
}

// removeMapKey deletes key (and its value) from a mapping node.
func removeMapKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}
