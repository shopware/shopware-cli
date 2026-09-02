package docker

import "gopkg.in/yaml.v3"

// composeFile mirrors only the subset of the compose spec the generator emits.
// Struct field order keeps the output stable; yamlMap keeps map-like sections
// (services, environment, depends_on, ...) in insertion order.
type composeFile struct {
	Services yamlMap[composeService]         `yaml:"services"`
	Volumes  yamlMap[struct{}]               `yaml:"volumes,omitempty"`
	Networks yamlMap[composeExternalNetwork] `yaml:"networks,omitempty"`
}

type composeService struct {
	Image       string                         `yaml:"image"`
	User        string                         `yaml:"user,omitempty"`
	Entrypoint  []string                       `yaml:"entrypoint,omitempty"`
	Command     []string                       `yaml:"command,omitempty"`
	Ports       []string                       `yaml:"ports,omitempty"`
	EnvFile     []string                       `yaml:"env_file,omitempty"`
	Environment yamlMap[string]                `yaml:"environment,omitempty"`
	Volumes     []string                       `yaml:"volumes,omitempty"`
	DependsOn   yamlMap[composeDependency]     `yaml:"depends_on,omitempty"`
	Healthcheck *composeHealthcheck            `yaml:"healthcheck,omitempty"`
	Restart     string                         `yaml:"restart,omitempty"`
	StopSignal  string                         `yaml:"stop_signal,omitempty"`
	Labels      yamlMap[string]                `yaml:"labels,omitempty"`
	Networks    yamlMap[composeServiceNetwork] `yaml:"networks,omitempty"`
}

// composeServiceNetwork is a service's per-network config; a project-unique
// alias keeps parallel projects from colliding on the bare service name on the
// shared proxy network (issue #1484).
type composeServiceNetwork struct {
	Aliases []string `yaml:"aliases,omitempty"`
}

// composeDependency is the long-form depends_on entry; an empty Condition
// means compose's default "service_started".
type composeDependency struct {
	Condition string `yaml:"condition,omitempty"`
}

type composeHealthcheck struct {
	Test          []string `yaml:"test"`
	StartPeriod   string   `yaml:"start_period,omitempty"`
	StartInterval string   `yaml:"start_interval,omitempty"`
	Interval      string   `yaml:"interval,omitempty"`
	Timeout       string   `yaml:"timeout,omitempty"`
	Retries       int      `yaml:"retries,omitempty"`
}

type composeExternalNetwork struct {
	External bool `yaml:"external"`
}

// yamlEntry is one key/value pair of a yamlMap.
type yamlEntry[T any] struct {
	Key   string
	Value T
}

// yamlMap is an insertion-ordered string-keyed map for stable yaml.Marshal
// output, where a plain Go map would shuffle keys on every regeneration.
type yamlMap[T any] []yamlEntry[T]

// set appends a key/value pair and returns the extended map, so entries can be
// chained in emission order.
func (m yamlMap[T]) set(key string, value T) yamlMap[T] {
	return append(m, yamlEntry[T]{Key: key, Value: value})
}

func (m yamlMap[T]) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, entry := range m {
		key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.Key}
		var value yaml.Node
		if err := value.Encode(entry.Value); err != nil {
			return nil, err
		}
		// Quote strings that contain YAML indicators (e.g. `&` in a Redis
		// messenger DSN) so compose parsers do not treat them as aliases.
		if s, ok := any(entry.Value).(string); ok && needsYAMLQuotes(s) {
			value.Style = yaml.DoubleQuotedStyle
		}
		node.Content = append(node.Content, key, &value)
	}
	return node, nil
}

func needsYAMLQuotes(s string) bool {
	for _, r := range s {
		if r == '&' || r == '*' || r == '!' {
			return true
		}
	}
	return false
}
