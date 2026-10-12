// Package installer delegates skill installation and removal to Vercel's skills CLI.
package installer

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/ai/directory"
)

type Options struct {
	Client string
	Global bool
	DryRun bool
}

// Result describes the delegated operation. A dry run does not invoke npx.
type Result struct {
	Version   int      `json:"version"`
	Action    string   `json:"action"`
	Name      string   `json:"name"`
	Client    string   `json:"client"`
	Scope     string   `json:"scope"`
	Directory string   `json:"directory"`
	Command   []string `json:"command"`
	DryRun    bool     `json:"dryRun"`
}

// Run adds or removes one curated skill. An omitted client delegates agent
// detection to skills. Installation state is owned by the skills CLI.
// Child output goes to output (stderr in the command), keeping JSON stdout clean.
func Run(ctx context.Context, action, name string, opts Options, output io.Writer) (*Result, error) {
	return run(ctx, action, name, opts, output, execute)
}

type runner func(context.Context, string, []string, io.Writer, io.Writer) error

func run(ctx context.Context, action, name string, opts Options, output io.Writer, execute runner) (*Result, error) {
	if action != "add" && action != "remove" {
		return nil, fmt.Errorf("cannot manage skill: unknown action %q", action)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot locate project directory: %w", err)
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve project directory: %w", err)
	}
	scope := "project"
	if opts.Global {
		scope = "global"
	}
	integration, err := directory.Load().Info(name)
	if err != nil {
		return nil, fmt.Errorf("cannot %s skill: %w", action, err)
	}
	source := ""
	if action == "add" {
		source, err = skillSource(integration)
		if err != nil {
			return nil, err
		}
	}
	result := &Result{Version: 1, Action: action, Name: name, Client: opts.Client, Scope: scope, Directory: cwd, DryRun: opts.DryRun}
	result.Command = arguments(action, name, source, opts.Client, opts.Global)
	if opts.DryRun {
		return result, nil
	}
	if err := execute(ctx, cwd, result.Command, output, output); err != nil {
		return nil, fmt.Errorf("cannot %s skill %q: %w", action, name, err)
	}
	return result, nil
}

func skillSource(e *directory.Integration) (string, error) {
	if e.Repository != "" {
		return e.Repository, nil
	}
	return "", fmt.Errorf("cannot add skill %q: missing installation source", e.Name)
}

func arguments(action, name, source, client string, global bool) []string {
	args := []string{"npx", "--yes", "skills", action}
	if action == "add" {
		args = append(args, source, "--skill", name)
	} else {
		args = append(args, name)
	}
	if client != "" {
		args = append(args, "--agent", client)
	}
	args = append(args, "--yes")
	if global {
		args = append(args, "--global")
	}
	return args
}

func execute(ctx context.Context, cwd string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = cwd
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cannot run npx skills (requires Node.js and npm): %w", err)
	}
	return nil
}
