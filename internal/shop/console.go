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
		Name        string `json:"name"`
		Description string `json:"description"`
		Hidden      bool   `json:"hidden"`
		Definition  struct {
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

const (
	consoleCommandsCache  = "console_commands.json"
	composerCommandsCache = "composer_commands.json"
)

func commandListCachePath(projectRoot, name string) string {
	return path.Join(projectRoot, "var", "cache", name)
}

func ReadCachedConsoleCompletion(projectRoot string) (*ConsoleResponse, error) {
	return readCachedCommandList(projectRoot, consoleCommandsCache)
}

func ReadCachedComposerCompletion(projectRoot string) (*ConsoleResponse, error) {
	return readCachedCommandList(projectRoot, composerCommandsCache)
}

func readCachedCommandList(projectRoot, cacheFile string) (*ConsoleResponse, error) {
	bytes, err := os.ReadFile(commandListCachePath(projectRoot, cacheFile))
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
	return getCommandList(ctx, projectRoot, consoleCommandsCache, consoleCommand)
}

func GetComposerCompletion(ctx context.Context, projectRoot string, composerCommand ConsoleCommandFunc) (*ConsoleResponse, error) {
	return getCommandList(ctx, projectRoot, composerCommandsCache, composerCommand)
}

func getCommandList(ctx context.Context, projectRoot, cacheFile string, listCommand ConsoleCommandFunc) (*ConsoleResponse, error) {
	if resp, err := readCachedCommandList(projectRoot, cacheFile); err == nil {
		return resp, nil
	}

	cmd := listCommand(ctx, "list", "--format=json")
	cmd.Dir = projectRoot

	commandJson, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var resp ConsoleResponse

	if err := json.Unmarshal(commandJson, &resp); err != nil {
		return nil, err
	}

	if err := os.WriteFile(commandListCachePath(projectRoot, cacheFile), commandJson, 0o644); err != nil {
		return nil, err
	}

	return &resp, nil
}
