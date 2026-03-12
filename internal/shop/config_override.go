package shop

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/system"
)

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

// mergeLocalConfig reads a local override file and merges it into the base
// config map. It supports !reset and !override YAML tags.
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

	resetPaths := make(map[string]bool)
	overridePaths := make(map[string]bool)
	collectTaggedPaths(&localNode, "", resetPaths, overridePaths)

	var localMap map[string]any
	if err := localNode.Decode(&localMap); err != nil {
		return nil, fmt.Errorf("decoding local config %s: %w", localFileName, err)
	}

	if localMap == nil {
		return baseMap, nil
	}

	for p := range resetPaths {
		deleteAtPath(baseMap, p)
	}

	mergeMap(baseMap, localMap, "", overridePaths)

	return baseMap, nil
}

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

			switch valNode.Tag {
			case "!reset":
				resetPaths[childPath] = true
				valNode.Tag = ""
			case "!override":
				overridePaths[childPath] = true
				valNode.Tag = ""
			}

			collectTaggedPaths(valNode, childPath, resetPaths, overridePaths)
		}
	case yaml.SequenceNode:
		switch node.Tag {
		case "!reset":
			resetPaths[currentPath] = true
			node.Tag = ""
		case "!override":
			overridePaths[currentPath] = true
			node.Tag = ""
		}

		for i, child := range node.Content {
			collectTaggedPaths(child, fmt.Sprintf("%s[%d]", currentPath, i), resetPaths, overridePaths)
		}
	case yaml.ScalarNode, yaml.AliasNode:
	}
}

func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}

	return parent + "." + key
}

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

func mergeMap(dst, src map[string]any, currentPath string, overridePaths map[string]bool) {
	for key, srcVal := range src {
		childPath := joinPath(currentPath, key)

		if overridePaths[childPath] {
			dst[key] = srcVal
			continue
		}

		dstVal, exists := dst[key]
		if !exists {
			dst[key] = srcVal
			continue
		}

		dstMap, dstIsMap := dstVal.(map[string]any)
		srcMap, srcIsMap := srcVal.(map[string]any)

		if dstIsMap && srcIsMap {
			mergeMap(dstMap, srcMap, childPath, overridePaths)
			continue
		}

		dstSlice, dstIsSlice := dstVal.([]any)
		srcSlice, srcIsSlice := srcVal.([]any)

		if dstIsSlice && srcIsSlice {
			dst[key] = append(dstSlice, srcSlice...)
			continue
		}

		dst[key] = srcVal
	}
}

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

func marshalMap(m map[string]any) ([]byte, error) {
	return yaml.Marshal(m)
}
