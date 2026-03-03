package shop

import (
	"context"
	"encoding/json"
	"fmt"
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

func (c ConsoleResponse) GetCommandOptions(name string) []string {
	for _, command := range c.Commands {
		if !command.Hidden && command.Name == name {
			options := make([]string, 0)
			for optionName := range command.Definition.Options {
				options = append(options, fmt.Sprintf("--%s", optionName))
			}

			return options
		}
	}
	return nil
}

// ConsoleCommandFunc is a function that creates a console command.
// This avoids a circular dependency between shop and executor packages.
type ConsoleCommandFunc func(ctx context.Context, args ...string) *exec.Cmd

func GetConsoleCompletion(ctx context.Context, projectRoot string, consoleCommand ConsoleCommandFunc) (*ConsoleResponse, error) {
	cachePath := path.Join(projectRoot, "var", "cache", "console_commands.json")

	if _, err := os.Stat(cachePath); err == nil {
		var resp ConsoleResponse

		bytes, err := os.ReadFile(cachePath)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(bytes, &resp); err != nil {
			return nil, err
		}

		return &resp, nil
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

	if err := os.WriteFile(cachePath, commandJson, os.ModePerm); err != nil {
		return nil, err
	}

	return &resp, nil
}
