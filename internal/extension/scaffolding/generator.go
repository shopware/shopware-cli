package scaffolding

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// PluginInfo describes the existing plugin a generator writes into.
type PluginInfo struct {
	// Dir is the absolute path of the plugin.
	Dir string
	// Namespace is the PHP namespace of the plugin, e.g. Swag\BasicExample.
	Namespace string
	// ClassName is the plugin class without its namespace, e.g. SwagBasicExample.
	ClassName string
}

// Generator creates an example implementation of a single Shopware feature
// inside an existing plugin. Generators are additive: they create missing files
// and extend the service and route configuration, but never touch code that is
// already there.
type Generator struct {
	// Name is the sub command name, e.g. "event-subscriber".
	Name string
	// Short describes the generator in the command list.
	Short string
	// Args documents the positional arguments, e.g. "ENTITY...".
	// It is empty for generators that take no arguments.
	Args string

	build func(plugin PluginInfo, args []string) (output, error)
}

// Result lists the paths a generator run touched, relative to the plugin.
type Result struct {
	// Created are files that did not exist before.
	Created []string
	// Updated are config files that gained a new block.
	Updated []string
	// Skipped are files that were already there and stayed untouched.
	Skipped []string
}

// Run executes the generator inside the plugin.
func (g Generator) Run(plugin PluginInfo, args []string) (Result, error) {
	var result Result

	out, err := g.build(plugin, args)
	if err != nil {
		return result, err
	}

	for _, f := range out.Files {
		state, err := createFile(plugin.Dir, f)
		if err != nil {
			return result, fmt.Errorf("create %s: %w", f.Path, err)
		}

		result.record(f.Path, state)
	}

	for _, s := range out.Snippets {
		state, err := applySnippet(plugin.Dir, s)
		if err != nil {
			return result, fmt.Errorf("update %s: %w", s.Path, err)
		}

		result.record(s.Path, state)
	}

	return result, nil
}

// templateData holds every value the generator stubs can reference. The entity
// fields are only filled by the entity generator.
type templateData struct {
	Namespace  string
	ClassName  string
	EntityName string
	TableName  string
	Timestamp  string
}

// file is a single file a generator creates from an embedded stub.
type file struct {
	// Path is relative to the plugin directory and always uses forward slashes.
	Path string
	Stub string
	// Raw copies the stub verbatim instead of rendering it. Twig stubs need
	// this because Twig and Go templates share the {{ }} delimiters.
	Raw  bool
	Data templateData
}

// snippet is a block of code added to a config file shared by all generators.
// A missing file is created from Intro and Outro, an existing one only gains
// the block. Content that is already present is never added twice.
type snippet struct {
	// Path is relative to the plugin directory and always uses forward slashes.
	Path    string
	Content string
	Intro   string
	Outro   string
}

// output is everything a single generator contributes to a plugin.
type output struct {
	Files    []file
	Snippets []snippet
}

// fileState tells what happened to a file during a generator run.
type fileState int

const (
	skipped fileState = iota
	created
	updated
)

func (r *Result) record(path string, state fileState) {
	switch state {
	case created:
		r.Created = append(r.Created, path)
	case updated:
		r.Updated = append(r.Updated, path)
	case skipped:
		r.Skipped = append(r.Skipped, path)
	}
}

// createFile renders a stub into the plugin, unless the file already exists.
func createFile(pluginDir string, f file) (fileState, error) {
	dest := filepath.Join(pluginDir, filepath.FromSlash(f.Path))

	_, err := os.Stat(dest)
	if err == nil {
		return skipped, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return skipped, fmt.Errorf("stat file: %w", err)
	}

	content, err := renderStub(f)
	if err != nil {
		return skipped, err
	}

	if err := writeFile(dest, content); err != nil {
		return skipped, err
	}

	return created, nil
}

// applySnippet adds a block to a config file without losing its current content.
func applySnippet(pluginDir string, s snippet) (fileState, error) {
	dest := filepath.Join(pluginDir, filepath.FromSlash(s.Path))

	existing, err := os.ReadFile(dest)
	if errors.Is(err, os.ErrNotExist) {
		if err := writeFile(dest, s.Intro+s.Content+s.Outro); err != nil {
			return skipped, err
		}

		return created, nil
	}
	if err != nil {
		return skipped, fmt.Errorf("read file: %w", err)
	}

	content := string(existing)
	if strings.Contains(content, s.Content) {
		return skipped, nil
	}

	// The block of a PHP config file belongs inside the returned closure, so it
	// goes in front of the closing "};" rather than at the end of the file.
	if marker := strings.TrimSpace(s.Outro); marker != "" {
		if at := strings.LastIndex(content, marker); at >= 0 {
			content = content[:at] + s.Content + content[at:]

			if err := writeFile(dest, content); err != nil {
				return skipped, err
			}

			return updated, nil
		}
	}

	if err := writeFile(dest, content+s.Content); err != nil {
		return skipped, err
	}

	return updated, nil
}

func renderStub(f file) (string, error) {
	stub, err := stubsFS.ReadFile(f.Stub)
	if err != nil {
		return "", fmt.Errorf("read stub: %w", err)
	}

	if f.Raw {
		return string(stub), nil
	}

	tmpl, err := template.New(f.Path).Parse(string(stub))
	if err != nil {
		return "", fmt.Errorf("parse stub: %w", err)
	}

	var rendered strings.Builder
	if err := tmpl.Execute(&rendered, f.Data); err != nil {
		return "", fmt.Errorf("render stub: %w", err)
	}

	return rendered.String(), nil
}

func writeFile(dest, content string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create subdirectories: %w", err)
	}

	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
