package shop

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/system"
)

// localConfigFileName returns the local override file path for a given config file.
// e.g., ".shopware-project.yml" -> ".shopware-project.local.yml"
func localConfigFileName(fileName string) string {
	ext := ""
	base := fileName

	if strings.HasSuffix(fileName, ".yaml") {
		ext = ".yaml"
		base = strings.TrimSuffix(fileName, ".yaml")
	} else if strings.HasSuffix(fileName, ".yml") {
		ext = ".yml"
		base = strings.TrimSuffix(fileName, ".yml")
	}

	return base + ".local" + ext
}

// mergeLocalConfig reads a local override file (if it exists) and merges it
// into the base config map. It supports !reset and !override YAML tags.
//
// !reset clears a field to its zero value (e.g., "ports: !reset []" results in empty list).
// !override replaces a field entirely instead of deep-merging.
func mergeLocalConfig(baseMap map[string]any, localFileName string) (map[string]any, error) {
	if _, err := os.Stat(localFileName); os.IsNotExist(err) {
		return baseMap, nil
	}

	localBytes, err := os.ReadFile(localFileName)
	if err != nil {
		return nil, fmt.Errorf("reading local config %s: %w", localFileName, err)
	}

	substituted := system.ExpandEnv(string(localBytes))

	var localNode yaml.Node
	if err := yaml.Unmarshal([]byte(substituted), &localNode); err != nil {
		return nil, fmt.Errorf("parsing local config %s: %w", localFileName, err)
	}

	if localNode.Kind == 0 {
		return baseMap, nil
	}

	// Collect paths with !reset and !override tags, then strip the tags
	resetPaths := make(map[string]bool)
	overridePaths := make(map[string]bool)
	collectTaggedPaths(&localNode, "", resetPaths, overridePaths)

	// Unmarshal the local config into a generic map
	var localMap map[string]any
	if err := localNode.Decode(&localMap); err != nil {
		return nil, fmt.Errorf("decoding local config %s: %w", localFileName, err)
	}

	if localMap == nil {
		return baseMap, nil
	}

	// Apply reset paths first: remove these keys from the base
	for p := range resetPaths {
		deleteAtPath(baseMap, p)
	}

	// Merge local into base, using override paths to force replacement
	mergeMap(baseMap, localMap, "", overridePaths)

	return baseMap, nil
}

// collectTaggedPaths walks a yaml.Node tree and records paths of nodes
// tagged with !reset or !override. It also strips the tags so normal
// decoding works.
func collectTaggedPaths(node *yaml.Node, currentPath string, resetPaths, overridePaths map[string]bool) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			collectTaggedPaths(child, currentPath, resetPaths, overridePaths)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			childPath := joinPath(currentPath, keyNode.Value)

			if valNode.Tag == "!reset" {
				resetPaths[childPath] = true
				valNode.Tag = ""
			} else if valNode.Tag == "!override" {
				overridePaths[childPath] = true
				valNode.Tag = ""
			}

			collectTaggedPaths(valNode, childPath, resetPaths, overridePaths)
		}
	case yaml.SequenceNode:
		if node.Tag == "!reset" {
			resetPaths[currentPath] = true
			node.Tag = ""
		} else if node.Tag == "!override" {
			overridePaths[currentPath] = true
			node.Tag = ""
		}

		for i, child := range node.Content {
			collectTaggedPaths(child, fmt.Sprintf("%s[%d]", currentPath, i), resetPaths, overridePaths)
		}
	}
}

func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}

	return parent + "." + key
}

// deleteAtPath removes the value at the given dot-separated path from a nested map.
func deleteAtPath(m map[string]any, path string) {
	parts := strings.Split(path, ".")
	current := m

	for i, part := range parts {
		if i == len(parts)-1 {
			delete(current, part)
			return
		}

		next, ok := current[part]
		if !ok {
			return
		}

		nextMap, ok := next.(map[string]any)
		if !ok {
			return
		}

		current = nextMap
	}
}

// mergeMap deep-merges src into dst. For paths in overridePaths,
// the src value replaces dst entirely. Otherwise:
// - maps are recursively merged
// - slices are appended
// - scalars from src override dst
func mergeMap(dst, src map[string]any, currentPath string, overridePaths map[string]bool) {
	for key, srcVal := range src {
		childPath := joinPath(currentPath, key)

		// If this path is marked as !override, replace entirely
		if overridePaths[childPath] {
			dst[key] = srcVal
			continue
		}

		dstVal, exists := dst[key]
		if !exists {
			dst[key] = srcVal
			continue
		}

		// Deep merge maps
		dstMap, dstIsMap := dstVal.(map[string]any)
		srcMap, srcIsMap := srcVal.(map[string]any)

		if dstIsMap && srcIsMap {
			mergeMap(dstMap, srcMap, childPath, overridePaths)
			continue
		}

		// Append slices
		dstSlice, dstIsSlice := dstVal.([]any)
		srcSlice, srcIsSlice := srcVal.([]any)

		if dstIsSlice && srcIsSlice {
			dst[key] = append(dstSlice, srcSlice...)
			continue
		}

		// Scalar override
		dst[key] = srcVal
	}
}

// readConfigAsMap reads a YAML config file and returns it as a generic map.
func readConfigAsMap(fileName string) (map[string]any, error) {
	fileHandle, err := os.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", fileName, err)
	}

	substituted := system.ExpandEnv(string(fileHandle))

	var m map[string]any
	if err := yaml.Unmarshal([]byte(substituted), &m); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", fileName, err)
	}

	if m == nil {
		m = make(map[string]any)
	}

	return m, nil
}

// marshalMap serializes a map back to YAML bytes.
func marshalMap(m map[string]any) ([]byte, error) {
	return yaml.Marshal(m)
}
