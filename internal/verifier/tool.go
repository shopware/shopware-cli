package verifier

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
)

type ToolList []Tool

var availableTools = ToolList{}

func AddTool(tool Tool) {
	availableTools = append(availableTools, tool)
}

func GetTools() ToolList {
	return availableTools
}

type ToolConfig struct {
	// Path to the tool directory
	ToolDirectory string

	InputWasDirectory bool

	// The minimum version of Shopware that is supported
	MinShopwareVersion string
	// The maximum version of Shopware that is supported
	MaxShopwareVersion string
	// The version of Shopware that is checked against
	CheckAgainst string
	// The root directory of the extension/project
	RootDir string
	// Contains a list of directories that are considered as source code
	SourceDirectories []string
	// Contains a list of identifiers that are ignored
	ValidationIgnores []validation.ToolConfigIgnore
	// Contains a list of directories that are considered as admin code
	AdminDirectories []string
	// Contains a list of directories that are considered as storefront code
	StorefrontDirectories []string

	Extension extension.Extension
}

type Tool interface {
	Name() string
	Check(ctx context.Context, check *Check, config ToolConfig) error
	Fix(ctx context.Context, config ToolConfig) error
	Format(ctx context.Context, config ToolConfig, dryRun bool) error
}

func (tl ToolList) Only(only string) (ToolList, error) {
	if only == "" {
		return tl, nil
	}

	var filteredTools []Tool
	requestedTools := strings.Split(only, ",")

	for _, requestedTool := range requestedTools {
		requestedTool = strings.TrimSpace(requestedTool)
		found := false

		for _, t := range tl {
			if t.Name() == requestedTool {
				filteredTools = append(filteredTools, t)
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("tool with name %q not found, possible tools: %s", requestedTool, tl.PossibleString())
		}
	}

	return filteredTools, nil
}

// Exclude filters out tools listed in the comma-separated exclude string.
// Returns an error if any specified tool name does not exist in the current list.
func (tl ToolList) Exclude(exclude string) (ToolList, error) {
	if exclude == "" {
		return tl, nil
	}

	requested := strings.Split(exclude, ",")

	// Validate all requested excludes exist
	for _, name := range requested {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		found := false
		for _, t := range tl {
			if t.Name() == name {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("tool with name %q not found, possible tools: %s", name, tl.PossibleString())
		}
	}

	// Build filtered list excluding requested names
	excludeSet := map[string]struct{}{}
	for _, name := range requested {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		excludeSet[name] = struct{}{}
	}

	var filtered ToolList
	for _, t := range tl {
		if _, ok := excludeSet[t.Name()]; ok {
			continue
		}
		filtered = append(filtered, t)
	}

	return filtered, nil
}

func (tl ToolList) PossibleString() string {
	var possibleTools []string
	for _, t := range tl {
		possibleTools = append(possibleTools, t.Name())
	}

	return strings.Join(possibleTools, ",")
}
