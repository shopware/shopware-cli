package shop

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path"
)

type ConsoleResponse struct {
	Commands []struct {
		Name       string `json:"name"`
		Hidden     bool   `json:"hidden"`
		Definition struct {
			Arguments interface{} `json:"arguments"`
			Options   map[string]struct {
				Shortcut string `json:"shortcut"`
			} `json:"options"`
		} `json:"definition"`
	} `json:"commands"`
}

func (c ConsoleResponse) HasCommand(name string) bool {
	for _, command := range c.Commands {
		if !command.Hidden && command.Name == name {
			return true
		}
	}
	return false
}

func (c ConsoleResponse) GetCommandOptions(name string) []string {
	for _, command := range c.Commands {
		if !command.Hidden && command.Name == name {
			options := make([]string, 0)
			for optionName := range command.Definition.Options {
				options = append(options, "--"+optionName)
			}

			return options
		}
	}
	return nil
}

// ConsoleCommandFunc avoids a circular dependency between shop and executor packages.
type ConsoleCommandFunc func(ctx context.Context, args ...string) *exec.Cmd

func consoleCompletionCachePath(projectRoot string) string {
	return path.Join(projectRoot, "var", "cache", "console_commands.json")
}

func ReadCachedConsoleCompletion(projectRoot string) (*ConsoleResponse, error) {
	bytes, err := os.ReadFile(consoleCompletionCachePath(projectRoot))
	if err != nil {
		return nil, err
	}

	var resp ConsoleResponse
	if err := json.Unmarshal(bytes, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func GetConsoleCompletion(ctx context.Context, projectRoot string, consoleCommand ConsoleCommandFunc) (*ConsoleResponse, error) {
	if resp, err := ReadCachedConsoleCompletion(projectRoot); err == nil {
		return resp, nil
	}

	cmd := consoleCommand(ctx, "list", "--format=json")
	cmd.Dir = projectRoot

	commandJson, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var resp ConsoleResponse

	if err := json.Unmarshal(commandJson, &resp); err != nil {
		return nil, err
	}

	if err := os.WriteFile(consoleCompletionCachePath(projectRoot), commandJson, 0o644); err != nil {
		return nil, err
	}

	return &resp, nil
}
