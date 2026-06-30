package symfony

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/envfile"
)

// BaseEnvironment is the pseudo-environment that represents configuration which
// is loaded for every environment (the files directly in config/packages and
// the when@<env> blocks are resolved relative to it).
const BaseEnvironment = "_base"

// whenPrefix is the Symfony key prefix used to scope a block of configuration to
// a single environment from within a regular config file (e.g. "when@dev").
const whenPrefix = "when@"

// ConfigFile is a single parsed YAML file below config/packages. It keeps the
// original document node so writes can preserve comments and formatting. Files
// directly in config/packages use BaseEnvironment as Environment; files in
// config/packages/<env>/ use that <env>.
type ConfigFile struct {
	Path        string
	Environment string
	doc         *yaml.Node
}

// ProjectConfig is the entry point for reading and mutating the config/packages
// tree of a Symfony project. Use NewProjectConfig to load it from disk.
type ProjectConfig struct {
	projectRoot string
	packagesDir string
	files       []*ConfigFile
}

// NewProjectConfig discovers and parses every YAML file below
// <projectRoot>/config/packages, including environment subdirectories. It does
// not resolve or merge anything yet; call Environments, Config or related
// methods for that. A project without a config/packages directory yields an
// empty ProjectConfig rather than an error.
func NewProjectConfig(projectRoot string) (*ProjectConfig, error) {
	packagesDir := filepath.Join(projectRoot, "config", "packages")

	pc := &ProjectConfig{
		projectRoot: projectRoot,
		packagesDir: packagesDir,
	}

	info, err := os.Stat(packagesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return pc, nil
		}

		return nil, err
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", packagesDir)
	}

	if err := pc.load(); err != nil {
		return nil, err
	}

	return pc, nil
}

// load walks config/packages and parses all *.yaml / *.yml files. Files in the
// root belong to BaseEnvironment, files in a direct subdirectory belong to that
// subdirectory's environment. Nested directories deeper than one level are
// ignored, matching Symfony's loader which only globs one level deep.
func (pc *ProjectConfig) load() error {
	var files []*ConfigFile

	entries, err := os.ReadDir(pc.packagesDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			envName := entry.Name()
			envDir := filepath.Join(pc.packagesDir, envName)

			envFiles, err := readYAMLDir(envDir, envName)
			if err != nil {
				return err
			}

			files = append(files, envFiles...)

			continue
		}

		if !isYAMLFile(entry.Name()) {
			continue
		}

		file, err := parseConfigFile(filepath.Join(pc.packagesDir, entry.Name()), BaseEnvironment)
		if err != nil {
			return err
		}

		files = append(files, file)
	}

	sortConfigFiles(files)
	pc.files = files

	return nil
}

// Environments returns the sorted list of environments the project defines,
// excluding BaseEnvironment. An environment is recognised either through a
// config/packages/<env>/ directory or through a when@<env> block in any file.
func (pc *ProjectConfig) Environments() []string {
	seen := map[string]struct{}{}

	for _, file := range pc.files {
		if file.Environment != BaseEnvironment {
			seen[file.Environment] = struct{}{}
		}

		for _, env := range file.whenEnvironments() {
			seen[env] = struct{}{}
		}
	}

	envs := make([]string, 0, len(seen))
	for env := range seen {
		envs = append(envs, env)
	}

	sort.Strings(envs)

	return envs
}

// Files returns every parsed config file in load order. The returned slice is a
// copy; mutating it does not affect the ProjectConfig.
func (pc *ProjectConfig) Files() []*ConfigFile {
	out := make([]*ConfigFile, len(pc.files))
	copy(out, pc.files)

	return out
}

// Config resolves the fully merged configuration for the given environment. The
// result is keyed by package name (e.g. "framework", "doctrine") and contains
// plain Go values (map[string]any, []any, scalars) so callers can inspect or
// transform arbitrary keys.
//
// Merge order follows Symfony, lowest to highest precedence:
//  1. config/packages/*.yaml          (base files)
//  2. when@<env> blocks in base files (resolved in base-file order)
//  3. config/packages/<env>/*.yaml    (environment files)
//  4. when@<env> blocks in environment files
//
// Maps are merged recursively; sequences and scalars from a higher-precedence
// source replace the lower-precedence value entirely.
func (pc *ProjectConfig) Config(environment string) (map[string]any, error) {
	merged := map[string]any{}

	for _, file := range pc.files {
		if file.Environment != BaseEnvironment && file.Environment != environment {
			continue
		}

		root, err := file.decodeRoot()
		if err != nil {
			return nil, fmt.Errorf("decoding %s: %w", file.Path, err)
		}

		for key, value := range root {
			if env, ok := whenEnvFromKey(key); ok {
				if env != environment {
					continue
				}

				if block, ok := value.(map[string]any); ok {
					merged = mergeValue(merged, block).(map[string]any)
				}

				continue
			}

			merged = mergeValue(merged, map[string]any{key: value}).(map[string]any)
		}
	}

	return merged, nil
}

// PackageConfig is a convenience wrapper around Config that returns the merged
// configuration of a single package for an environment. The second return value
// reports whether the package is configured at all.
func (pc *ProjectConfig) PackageConfig(environment, pkg string) (any, bool, error) {
	cfg, err := pc.Config(environment)
	if err != nil {
		return nil, false, err
	}

	value, ok := cfg[pkg]

	return value, ok, nil
}

// Env returns the project's environment variables parsed from its Symfony env
// files (.env.dist < .env < .env.local), merged with Symfony precedence. The
// returned map is the same source ResolvedConfig uses to resolve %env(...)%
// references. An empty map is returned when the project has no env files.
func (pc *ProjectConfig) Env() (map[string]string, error) {
	return envfile.ReadAll(pc.projectRoot)
}

// GetConfigValue resolves a single dotted path (rooted at the package name,
// e.g. "framework.cache.app") in the merged configuration of an environment.
// The second return value reports whether the path exists.
func (pc *ProjectConfig) GetConfigValue(environment, path string) (any, bool, error) {
	cfg, err := pc.Config(environment)
	if err != nil {
		return nil, false, err
	}

	segments := splitPath(path)
	if len(segments) == 0 {
		return nil, false, fmt.Errorf("empty config path")
	}

	var current any = cfg

	for _, segment := range segments {
		m, ok := asStringMap(current)
		if !ok {
			return nil, false, nil
		}

		value, ok := m[segment]
		if !ok {
			return nil, false, nil
		}

		current = value
	}

	return current, true, nil
}

// whenEnvironments returns the environments referenced through when@<env> keys
// at the root of the file.
func (f *ConfigFile) whenEnvironments() []string {
	root := f.rootMapping()
	if root == nil {
		return nil
	}

	var envs []string

	for i := 0; i+1 < len(root.Content); i += 2 {
		if env, ok := whenEnvFromKey(root.Content[i].Value); ok {
			envs = append(envs, env)
		}
	}

	return envs
}

// decodeRoot decodes the file's root mapping into plain Go values.
func (f *ConfigFile) decodeRoot() (map[string]any, error) {
	root := f.rootMapping()
	if root == nil {
		return map[string]any{}, nil
	}

	var out map[string]any
	if err := root.Decode(&out); err != nil {
		return nil, err
	}

	if out == nil {
		out = map[string]any{}
	}

	return out, nil
}

// rootMapping returns the top-level mapping node of the file, or nil when the
// file is empty or not a mapping.
func (f *ConfigFile) rootMapping() *yaml.Node {
	if f.doc == nil || len(f.doc.Content) == 0 {
		return nil
	}

	root := f.doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}

	return root
}

// parseConfigFile reads and parses a single YAML config file.
func parseConfigFile(path, environment string) (*ConfigFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	file := &ConfigFile{Path: path, Environment: environment}

	if len(strings.TrimSpace(string(content))) == 0 {
		return file, nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	file.doc = &doc

	return file, nil
}

// readYAMLDir parses all top-level YAML files in dir, assigning them to
// environment.
func readYAMLDir(dir, environment string) ([]*ConfigFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []*ConfigFile

	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}

		file, err := parseConfigFile(filepath.Join(dir, entry.Name()), environment)
		if err != nil {
			return nil, err
		}

		files = append(files, file)
	}

	return files, nil
}

// sortConfigFiles orders files so that the merge in Config follows Symfony's
// precedence: base files first, then environment files, alphabetically by path
// within each group for determinism.
func sortConfigFiles(files []*ConfigFile) {
	sort.SliceStable(files, func(i, j int) bool {
		ei, ej := files[i].Environment == BaseEnvironment, files[j].Environment == BaseEnvironment
		if ei != ej {
			return ei // base environment sorts first
		}

		return files[i].Path < files[j].Path
	})
}

// whenEnvFromKey returns the environment encoded in a when@<env> key.
func whenEnvFromKey(key string) (string, bool) {
	if env, ok := strings.CutPrefix(key, whenPrefix); ok && env != "" {
		return env, true
	}

	return "", false
}

// isYAMLFile reports whether name has a YAML extension.
func isYAMLFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))

	return ext == ".yaml" || ext == ".yml"
}
