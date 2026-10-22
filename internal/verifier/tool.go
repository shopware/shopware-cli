package verifier

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

type ToolList[T Tool] []T

var availableTools = ToolList[Tool]{}

func AddTool(tool Tool) {
	availableTools = append(availableTools, tool)
}

func GetTools() ToolList[Tool] {
	return availableTools
}

// GetToolsOf returns registered tools that implement the requested capability.
func GetToolsOf[T Tool]() ToolList[T] {
	var tools ToolList[T]
	for _, tool := range availableTools {
		if casted, ok := tool.(T); ok {
			tools = append(tools, casted)
		}
	}
	return tools
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
	// List of extensions of the project or a single extension
	Extensions []extension.Extension
	// PHP version of the project the extensions are validated in; empty for a single extension
	PHPVersion string
}

func (c *ToolConfig) logConfiguration(ctx context.Context) {
	logger := logging.FromContext(ctx)
	logger.Debugf("Tool root directory: %s", c.RootDir)
	for _, dir := range c.SourceDirectories {
		logger.Debugf("Tool source directory: %s", dir)
	}
	for _, dir := range c.AdminDirectories {
		logger.Debugf("Tool administration directory: %s", dir)
	}
	for _, dir := range c.StorefrontDirectories {
		logger.Debugf("Tool storefront directory: %s", dir)
	}
	for _, ext := range c.Extensions {
		name, err := ext.GetName()
		if err != nil {
			logger.Debugf("Cannot read tool extension name for %s: %v", ext.GetPath(), err)
			continue
		}
		logger.Debugf("Tool extension: %s", name)
	}
}

type Tool interface {
	Name() string
}

type CheckTool interface {
	Tool
	Check(ctx context.Context, config ToolConfig) ToolResult
}

type FixTool interface {
	Tool
	Fix(ctx context.Context, config ToolConfig) ToolResult
}

type FormatTool interface {
	Tool
	Format(ctx context.Context, config ToolConfig, dryRun bool) ToolResult
}

func canonicalToolName(name string) string {
	if name == "sw-cli" {
		return "builtin"
	}

	return name
}

// WarnOnDeprecatedToolName reports use of the legacy built-in checker name.
func WarnOnDeprecatedToolName(ctx context.Context, values ...string) {
	for _, value := range values {
		for _, name := range strings.Split(value, ",") {
			if strings.TrimSpace(name) == "sw-cli" {
				logging.FromContext(ctx).Warnf("The tool name %q is deprecated as input; use %q instead", "sw-cli", "builtin")
				return
			}
		}
	}
}

func (tl ToolList[T]) Only(only string) (ToolList[T], error) {
	if only == "" {
		return tl, nil
	}

	var filteredTools ToolList[T]
	requestedTools := strings.Split(only, ",")
	seen := make(map[string]bool, len(requestedTools))

	for _, requestedTool := range requestedTools {
		requestedTool = canonicalToolName(strings.TrimSpace(requestedTool))
		found := false

		for _, t := range tl {
			if t.Name() == requestedTool {
				if !seen[requestedTool] {
					filteredTools = append(filteredTools, t)
					seen[requestedTool] = true
				}
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
func (tl ToolList[T]) Exclude(exclude string) (ToolList[T], error) {
	if exclude == "" {
		return tl, nil
	}

	names := strings.Split(exclude, ",")
	for i, name := range names {
		name = canonicalToolName(strings.TrimSpace(name))
		names[i] = name
		if name == "" {
			continue
		}
		if !slices.ContainsFunc(tl, func(tool T) bool { return tool.Name() == name }) {
			return nil, fmt.Errorf("tool with name %q not found in the selected tools: %s (--exclude only removes selected tools)", name, tl.PossibleString())
		}
	}

	var filtered ToolList[T]
	for _, t := range tl {
		if !slices.Contains(names, t.Name()) {
			filtered = append(filtered, t)
		}
	}

	return filtered, nil
}

// SelectTools applies the --only and --exclude filters and returns the
// selected tools and results for skipped tools. kind names the tools in the
// error returned when no tools remain, such as "formatters".
func SelectTools[T Tool](all ToolList[T], only, exclude, kind string) (
	ToolList[T],
	[]ToolResult,
	error,
) {
	requested, err := all.Only(only)
	if err != nil {
		return nil, nil, err
	}
	selected, err := requested.Exclude(exclude)
	if err != nil {
		return nil, nil, err
	}
	if len(selected) == 0 {
		return nil, nil, fmt.Errorf("no %s selected after applying --exclude", kind)
	}

	return selected, skippedTools(all, requested, selected), nil
}

func skippedTools[T Tool](all, requested, selected ToolList[T]) []ToolResult {
	results := make([]ToolResult, 0, len(all))
	for _, tool := range all {
		result := ToolResult{Name: tool.Name(), SkipReason: "not selected by --only"}
		switch {
		case slices.ContainsFunc(selected, func(candidate T) bool { return candidate.Name() == tool.Name() }):
			continue
		case slices.ContainsFunc(requested, func(candidate T) bool { return candidate.Name() == tool.Name() }):
			result.SkipReason = "excluded by --exclude"
		}
		results = append(results, result)
	}
	return results
}

func (tl ToolList[T]) PossibleString() string {
	var possibleTools []string
	for _, t := range tl {
		possibleTools = append(possibleTools, t.Name())
	}

	return strings.Join(possibleTools, ",")
}

// CompleteNames completes a comma-separated tool selection, skipping names already typed.
func (tl ToolList[T]) CompleteNames(toComplete string) []string {
	prefix := ""
	if i := strings.LastIndex(toComplete, ","); i >= 0 {
		prefix = toComplete[:i+1]
	}

	var chosen []string
	for _, name := range strings.Split(prefix, ",") {
		chosen = append(chosen, canonicalToolName(strings.TrimSpace(name)))
	}

	var completions []string
	for _, t := range tl {
		if !slices.Contains(chosen, t.Name()) {
			completions = append(completions, prefix+t.Name())
		}
	}

	return completions
}

// CommandContext logs the command before creating it with exec.CommandContext.
func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	logging.FromContext(ctx).Debugf("Executing command: %s %s", name, strings.Join(arg, " "))
	return exec.CommandContext(ctx, name, arg...)
}

// captureCommand keeps the streams separate, including output from failed commands.
func captureCommand(cmd *exec.Cmd, result *ToolResult) ([]byte, []byte, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result.Stdout += stdout.String()
	result.Stderr += stderr.String()
	return stdout.Bytes(), stderr.Bytes(), err
}

func expectedExit(err error, codes ...int) bool {
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && slices.Contains(codes, exitErr.ExitCode())
}

// commandOutputOrError prefers stderr, then stdout, and falls back to the process error.
// Captured output is trimmed before it is returned.
func commandOutputOrError(stderr string, stdout []byte, err error) string {
	if output := strings.TrimSpace(stderr); output != "" {
		return output
	}
	if output := strings.TrimSpace(string(stdout)); output != "" {
		return output
	}
	if err != nil {
		return err.Error()
	}

	return ""
}
