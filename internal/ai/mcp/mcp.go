// Package mcp configures MCP servers for a project by maintaining a standard
// .mcp.json file in the project root. The file uses the conventional
// {"mcpServers": {...}} shape that MCP clients read, so a single committed file
// serves the whole project (and the team) without the CLI having to know each
// agent's own config location. Because the CLI owns this one file, add and remove
// target exactly the named server.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileName is the standard MCP config file an MCP client reads from a project.
const FileName = ".mcp.json"

// serversKey is the top-level object that holds the servers.
const serversKey = "mcpServers"

// Installed is one MCP server configured in a project's .mcp.json.
type Installed struct {
	Name   string
	Source string // the server URL
}

// ConfigPath returns the .mcp.json path for the given project root.
func ConfigPath(dir string) string {
	return filepath.Join(dir, FileName)
}

// Add writes the server into dir's .mcp.json under the standard mcpServers key,
// replacing any entry of the same name. Other servers and unrelated keys are
// preserved.
func Add(dir, name, url, transport string) error {
	path := ConfigPath(dir)

	doc, err := readDoc(path)
	if err != nil {
		return err
	}
	servers := serversMap(doc)
	servers[name] = entry(url, transport)
	doc[serversKey] = servers

	return writeDoc(path, doc)
}

// Has reports whether the named server is configured in dir's .mcp.json, without
// changing anything. Callers use it to report truthfully on a dry run.
func Has(dir, name string) (bool, error) {
	doc, err := readDoc(ConfigPath(dir))
	if err != nil {
		return false, err
	}
	servers, _ := doc[serversKey].(map[string]any)
	_, exists := servers[name]

	return exists, nil
}

// Remove deletes exactly the named server from dir's .mcp.json and reports
// whether it was there.
func Remove(dir, name string) (bool, error) {
	path := ConfigPath(dir)

	doc, err := readDoc(path)
	if err != nil {
		return false, err
	}
	servers, _ := doc[serversKey].(map[string]any)
	if _, exists := servers[name]; !exists {
		return false, nil
	}
	delete(servers, name)
	doc[serversKey] = servers

	return true, writeDoc(path, doc)
}

// List returns the MCP servers configured in dir's .mcp.json, sorted by name.
func List(dir string) ([]Installed, error) {
	doc, err := readDoc(ConfigPath(dir))
	if err != nil {
		return nil, err
	}
	servers, _ := doc[serversKey].(map[string]any)

	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Installed, 0, len(names))
	for _, name := range names {
		out = append(out, Installed{Name: name, Source: sourceOf(servers[name])})
	}

	return out, nil
}

// entry is the standard remote-server shape: {"type":"http","url":...}.
func entry(url, transport string) map[string]any {
	if transport == "" {
		transport = "http"
	}

	return map[string]any{"type": transport, "url": url}
}

// sourceOf extracts the server URL from an entry.
func sourceOf(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if url, ok := m["url"].(string); ok {
		return url
	}

	return ""
}

func serversMap(doc map[string]any) map[string]any {
	if m, ok := doc[serversKey].(map[string]any); ok {
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

// writeDoc writes doc as indented JSON to path atomically: it writes a temporary
// file in the same directory and renames it over the target, so a reader never
// sees a half-written config and an interrupted write cannot corrupt it. A path
// that is a symlink is resolved first so the rename updates the link's target
// rather than replacing the link, and an existing file's mode is preserved.
func writeDoc(path string, doc map[string]any) error {
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	mode := os.FileMode(0o644)
	if fi, err := os.Stat(target); err == nil {
		mode = fi.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()

		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()

		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), target)
}
