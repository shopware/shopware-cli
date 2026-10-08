// Package mcp is a thin client over the add-mcp CLI (via npx): it builds the
// argv to install and remove MCP servers in AI agents. add-mcp owns the
// per-agent configuration; this package only shells out to it, mirroring the
// skills package.
package mcp

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// addMCPVersion pins the add-mcp major line; npx resolves the latest 2.x, so we
// get fixes but never a breaking 3.0.
const addMCPVersion = "2"

// AddArgs builds the `npx add-mcp <url> ...` argv for a remote MCP server.
func AddArgs(url, name, agent, transport string, global, assumeYes bool) []string {
	argv := []string{
		"npx", "--yes", "add-mcp@" + addMCPVersion, url,
		"--name", name,
		"--agent", agent,
		"--transport", transport,
	}
	if global {
		argv = append(argv, "--global")
	}
	if assumeYes {
		argv = append(argv, "-y")
	}

	return argv
}

// RemoveArgs builds the `npx add-mcp remove <name> ...` argv.
func RemoveArgs(name, agent string, global bool) []string {
	argv := []string{
		"npx", "--yes", "add-mcp@" + addMCPVersion, "remove", name,
		"--agent", agent,
	}
	if global {
		argv = append(argv, "--global")
	}
	argv = append(argv, "-y")

	return argv
}

// Run runs an add-mcp command via npx in dir (empty = current directory),
// streaming output to out.
func Run(ctx context.Context, argv []string, dir string, out io.Writer) error {
	if _, err := exec.LookPath(argv[0]); err != nil {
		return fmt.Errorf("%s not found: installing MCP servers requires Node.js/npx on PATH", argv[0])
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("add-mcp failed: %w", err)
	}

	return nil
}
