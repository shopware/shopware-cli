// Package mcp configures remote MCP servers in AI agents by editing their config
// files directly (no external tooling), so add and remove target exactly the named
// server. It supports the agents Meteor documents; MCP installs are global.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Installed is one MCP server configured for an agent.
type Installed struct {
	Name   string
	Agent  string // the agent's display name
	Source string // the server URL
}

type entryShape int

const (
	shapeHTTP   entryShape = iota // {"type":"http","url":...} — direct remote server
	shapeBridge                   // {"command":"npx","args":["mcp-remote",url]} — stdio bridge
)

type agentConfig struct {
	display string
	key     string // top-level object holding servers ("mcpServers" or "servers")
	shape   entryShape
	path    func() (string, error) // the global config file
}

func home(parts ...string) func() (string, error) {
	return func() (string, error) {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		return filepath.Join(append([]string{h}, parts...)...), nil
	}
}

func userConfig(parts ...string) func() (string, error) {
	return func() (string, error) {
		// UserConfigDir is ~/Library/Application Support on macOS and
		// $XDG_CONFIG_HOME (or ~/.config) on Linux — matching where these agents keep
		// their config.
		d, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}

		return filepath.Join(append([]string{d}, parts...)...), nil
	}
}

// agents are the clients Meteor documents. All store servers as JSON; VS Code uses
// the "servers" key, and Claude Desktop needs an npx bridge for a remote server.
var agents = map[string]agentConfig{
	"claude-code":    {"Claude Code", "mcpServers", shapeHTTP, home(".claude.json")},
	"cursor":         {"Cursor", "mcpServers", shapeHTTP, home(".cursor", "mcp.json")},
	"vscode":         {"VS Code", "servers", shapeHTTP, userConfig("Code", "User", "mcp.json")},
	"claude-desktop": {"Claude Desktop", "mcpServers", shapeBridge, userConfig("Claude", "claude_desktop_config.json")},
}

// Supported reports whether agent has a known MCP config layout.
func Supported(agent string) bool {
	_, ok := agents[agent]

	return ok
}

// SupportedAgents lists the supported agent ids, sorted.
func SupportedAgents() []string {
	ids := make([]string, 0, len(agents))
	for id := range agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	return ids
}

// ConfigPath returns the file agent's MCP servers live in.
func ConfigPath(agent string) (string, error) {
	cfg, ok := agents[agent]
	if !ok {
		return "", unsupported(agent)
	}

	return cfg.path()
}

// Add writes the server into agent's config under the agent's server key,
// replacing any entry of the same name. Other config content is preserved.
func Add(agent, name, url, transport string) error {
	cfg, ok := agents[agent]
	if !ok {
		return unsupported(agent)
	}
	path, err := cfg.path()
	if err != nil {
		return err
	}

	doc, err := readDoc(path)
	if err != nil {
		return err
	}
	servers := serversMap(doc, cfg.key)
	servers[name] = cfg.entry(url, transport)
	doc[cfg.key] = servers

	return writeDoc(path, doc)
}

// Remove deletes exactly the named server from agent's config and reports whether
// it was there.
func Remove(agent, name string) (bool, error) {
	cfg, ok := agents[agent]
	if !ok {
		return false, unsupported(agent)
	}
	path, err := cfg.path()
	if err != nil {
		return false, err
	}

	doc, err := readDoc(path)
	if err != nil {
		return false, err
	}
	servers, _ := doc[cfg.key].(map[string]any)
	if _, exists := servers[name]; !exists {
		return false, nil
	}
	delete(servers, name)
	doc[cfg.key] = servers

	return true, writeDoc(path, doc)
}

// List returns the MCP servers configured across the supported agents.
func List() ([]Installed, error) {
	var out []Installed
	for _, id := range SupportedAgents() {
		cfg := agents[id]
		path, err := cfg.path()
		if err != nil {
			continue
		}
		doc, err := readDoc(path)
		if err != nil {
			continue // an unreadable config is skipped; listing is best-effort
		}

		servers, _ := doc[cfg.key].(map[string]any)
		names := make([]string, 0, len(servers))
		for name := range servers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			out = append(out, Installed{Name: name, Agent: cfg.display, Source: sourceOf(servers[name])})
		}
	}

	return out, nil
}

func (c agentConfig) entry(url, transport string) map[string]any {
	if c.shape == shapeBridge {
		return map[string]any{"command": "npx", "args": []any{"mcp-remote", url}}
	}
	if transport == "" {
		transport = "http"
	}

	return map[string]any{"type": transport, "url": url}
}

// sourceOf extracts the server URL from an entry (direct or npx bridge).
func sourceOf(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if url, ok := m["url"].(string); ok {
		return url
	}
	if args, ok := m["args"].([]any); ok && len(args) > 0 {
		if last, ok := args[len(args)-1].(string); ok {
			return last
		}
	}

	return ""
}

func serversMap(doc map[string]any, key string) map[string]any {
	if m, ok := doc[key].(map[string]any); ok {
		return m
	}

	return map[string]any{}
}

func readDoc(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return map[string]any{}, nil
	}

	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}

	return doc, nil
}

func writeDoc(path string, doc map[string]any) error {
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, b, 0o644)
}

func unsupported(agent string) error {
	return fmt.Errorf("agent %q is not supported for MCP (supported: %s)", agent, strings.Join(SupportedAgents(), ", "))
}
