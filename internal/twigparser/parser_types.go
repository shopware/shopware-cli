package twigparser

import (
	"errors"
	"strings"
)

// parseTypes parses the content of a types tag.
// For example, given "score: 'number'" it returns a TypesNode with the mapping.
func parseTypes(content string) (Node, error) {
	typesMap := make(map[string]string)
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("no types provided")
	}
	// For simplicity, assume tokens do not contain spaces.
	tokens := strings.Fields(content)
	for _, token := range tokens {
		parts := strings.SplitN(token, ":", 2)
		if len(parts) != 2 {
			return nil, errors.New("invalid types format")
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// Remove quotes if present.
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		// Preserve quotes in dump.
		typesMap[key] = "'" + value + "'"
	}
	return &TypesNode{Types: typesMap}, nil
}
