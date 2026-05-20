// Package symfonyconfig parses and merges Symfony-style YAML configuration
// from a project's config/packages directory.
//
// Files are loaded in the order Symfony's default MicroKernel uses:
//
//  1. config/packages/*.{yaml,yml}                    (all environments)
//  2. config/packages/<env>/*.{yaml,yml}              (env-specific overrides)
//  3. config/services.{yaml,yml}                      (optional)
//  4. config/services_<env>.{yaml,yml}                (optional)
//
// Files within each directory are loaded alphabetically. Later files merge
// into earlier ones using a recursive deep-merge where maps merge key by key
// and scalars / sequences are replaced ("later wins"). This matches the
// effective config the kernel sees for rule-style analysis.
package symfonyconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Options configures the loader.
type Options struct {
	// Env is the Symfony environment (dev/prod/test). Defaults to "dev".
	Env string
	// IncludeServices also loads config/services.yaml and config/services_<env>.yaml.
	IncludeServices bool
	// ExtraEnv supplies additional environment variables to make available to
	// %env(VAR)% resolution. They take precedence over the process environment.
	ExtraEnv map[string]string
}

// File describes a single YAML config file that contributed to the merged
// result. Used so callers (e.g. recommendation rules) can point at the
// source location of a value.
type File struct {
	Path string
	Node *yaml.Node
}

// Config is the merged result of loading a Symfony project.
type Config struct {
	// Data is the merged config tree as nested map[string]any / []any / scalars.
	Data map[string]any
	// Files is the ordered list of files that were loaded.
	Files []File
	// Env is the resolved Symfony environment used during loading.
	Env string
	// EnvVars holds the merged environment (.env + .env.local + .env.<env> + .env.<env>.local
	// + process env + Options.ExtraEnv). Used by Resolve().
	EnvVars map[string]string
}

// Load reads the Symfony configuration for the given project root.
func Load(projectRoot string, opts Options) (*Config, error) {
	env := opts.Env
	if env == "" {
		env = os.Getenv("APP_ENV")
	}
	if env == "" {
		env = "dev"
	}

	envVars, err := loadDotenv(projectRoot, env)
	if err != nil {
		return nil, err
	}
	for k, v := range opts.ExtraEnv {
		envVars[k] = v
	}

	cfg := &Config{
		Data:    map[string]any{},
		Env:     env,
		EnvVars: envVars,
	}

	files, err := collectFiles(projectRoot, env, opts.IncludeServices)
	if err != nil {
		return nil, err
	}

	for _, path := range files {
		data, node, err := readYAMLFile(path)
		if err != nil {
			return nil, err
		}
		if data == nil {
			cfg.Files = append(cfg.Files, File{Path: path, Node: node})
			continue
		}
		deepMerge(cfg.Data, data)
		cfg.Files = append(cfg.Files, File{Path: path, Node: node})
	}

	return cfg, nil
}

// collectFiles returns absolute paths to all relevant YAML files in load order.
func collectFiles(projectRoot, env string, includeServices bool) ([]string, error) {
	var files []string

	packagesDir := filepath.Join(projectRoot, "config", "packages")
	base, err := listYAMLFiles(packagesDir)
	if err != nil {
		return nil, err
	}
	files = append(files, base...)

	envDir := filepath.Join(packagesDir, env)
	envFiles, err := listYAMLFiles(envDir)
	if err != nil {
		return nil, err
	}
	files = append(files, envFiles...)

	if includeServices {
		for _, name := range []string{"services.yaml", "services.yml"} {
			path := filepath.Join(projectRoot, "config", name)
			if fileExists(path) {
				files = append(files, path)
				break
			}
		}
		for _, name := range []string{"services_" + env + ".yaml", "services_" + env + ".yml"} {
			path := filepath.Join(projectRoot, "config", name)
			if fileExists(path) {
				files = append(files, path)
				break
			}
		}
	}

	return files, nil
}

// listYAMLFiles returns *.yaml / *.yml files in dir, sorted alphabetically.
// Subdirectories are not descended into (Symfony only loads the top level of
// each directory; env overrides live in a dedicated subdirectory).
func listYAMLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}

	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Strings(out)
	return out, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readYAMLFile parses a YAML file and returns both the decoded map and the
// raw yaml.Node (so callers can recover line numbers for diagnostics).
func readYAMLFile(path string) (map[string]any, *yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil, nil
	}

	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	if node.Kind == 0 {
		return nil, &node, nil
	}

	var m map[string]any
	if err := node.Decode(&m); err != nil {
		return nil, &node, fmt.Errorf("decoding %s: %w", path, err)
	}
	return m, &node, nil
}

// deepMerge merges src into dst recursively.
//
// Semantics:
//   - Both values are maps -> recurse.
//   - Otherwise src replaces dst (sequences included).
//
// This matches "later wins" semantics for the structural merge analysis
// recommendation rules need.
func deepMerge(dst, src map[string]any) {
	for key, srcVal := range src {
		dstVal, exists := dst[key]
		if !exists {
			dst[key] = srcVal
			continue
		}

		dstMap, dstIsMap := dstVal.(map[string]any)
		srcMap, srcIsMap := srcVal.(map[string]any)
		if dstIsMap && srcIsMap {
			deepMerge(dstMap, srcMap)
			continue
		}

		dst[key] = srcVal
	}
}
