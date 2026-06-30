package symfony

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SetConfigValue sets the value at the given dotted path for an environment and
// persists the change to disk. The path is rooted at the package name, e.g.
// "framework.cache.app" or "doctrine.dbal.url".
//
// The target file is chosen to mirror where Symfony itself would read the value
// from, so existing comments, key order and formatting in unrelated parts of
// the file are preserved:
//
//   - If a file already defines the path (the deepest existing prefix wins),
//     that file is edited in place.
//   - Otherwise, for an environment other than BaseEnvironment that has a
//     config/packages/<env>/ directory, a file named <package>.yaml in that
//     directory is used (created if necessary).
//   - Otherwise config/packages/<package>.yaml is used (created if necessary).
//
// After a successful write the in-memory state is reloaded so subsequent reads
// observe the change.
func (pc *ProjectConfig) SetConfigValue(environment string, path string, value any) error {
	segments := splitPath(path)
	if len(segments) == 0 {
		return fmt.Errorf("empty config path")
	}

	target := pc.resolveWriteTarget(environment, segments)

	if err := target.set(segments, value); err != nil {
		return err
	}

	if err := target.file.save(); err != nil {
		return err
	}

	return pc.load()
}

// writeTarget is the resolved destination of a SetConfigValue: the file to edit
// and, when non-empty, the when@<whenEnv> block inside it the value belongs to.
type writeTarget struct {
	file    *ConfigFile
	whenEnv string
}

// set writes value into the target, descending into the when@<env> block first
// when the target points at one.
func (t writeTarget) set(segments []string, value any) error {
	if t.whenEnv == "" {
		return t.file.set(segments, value)
	}

	return t.file.setUnderWhen(t.whenEnv, segments, value)
}

// resolveWriteTarget picks where SetConfigValue should write for the given
// environment and path, creating an in-memory file handle when no existing file
// is suitable.
func (pc *ProjectConfig) resolveWriteTarget(environment string, segments []string) writeTarget {
	if target, ok := pc.fileDefiningPath(environment, segments); ok {
		return target
	}

	pkg := segments[0]

	if environment != BaseEnvironment && pc.hasEnvironmentDir(environment) {
		return writeTarget{file: &ConfigFile{
			Path:        filepath.Join(pc.packagesDir, environment, pkg+".yaml"),
			Environment: environment,
		}}
	}

	return writeTarget{file: &ConfigFile{
		Path:        filepath.Join(pc.packagesDir, pkg+".yaml"),
		Environment: BaseEnvironment,
	}}
}

// fileDefiningPath returns the highest-precedence loaded file (for the base or
// the given environment) that already defines the deepest possible prefix of
// segments. It prefers the most specific match so an override lands next to the
// value it overrides, and reports whether that match lives in the file's
// when@<env> block so the write stays scoped to the right environment.
func (pc *ProjectConfig) fileDefiningPath(environment string, segments []string) (writeTarget, bool) {
	var best writeTarget
	found := false
	bestDepth := 0

	// Iterate in load order so that, for an equal match depth, the
	// highest-precedence (latest) file wins.
	for _, file := range pc.files {
		if file.Environment != BaseEnvironment && file.Environment != environment {
			continue
		}

		depth, underWhen := file.definedDepth(environment, segments)
		if depth > 0 && depth >= bestDepth {
			whenEnv := ""
			if underWhen {
				whenEnv = environment
			}

			best = writeTarget{file: file, whenEnv: whenEnv}
			bestDepth = depth
			found = true
		}
	}

	return best, found
}

// hasEnvironmentDir reports whether config/packages/<environment>/ exists.
func (pc *ProjectConfig) hasEnvironmentDir(environment string) bool {
	info, err := os.Stat(filepath.Join(pc.packagesDir, environment))

	return err == nil && info.IsDir()
}

// definedDepth returns how many leading segments of path are present in the
// file and whether that deepest match lives in the file's when@<environment>
// block rather than at the document root. A depth of 0 means the file does not
// contribute to this path.
func (f *ConfigFile) definedDepth(environment string, segments []string) (int, bool) {
	root := f.rootMapping()
	if root == nil {
		return 0, false
	}

	depth := mappingDepth(root, segments)
	underWhen := false

	if when := mappingChild(root, whenPrefix+environment); when != nil {
		if d := mappingDepth(when, segments); d > depth {
			depth = d
			underWhen = true
		}
	}

	return depth, underWhen
}

// set writes value at the dotted path inside the file, creating intermediate
// mappings as needed. The document and its comments are preserved.
func (f *ConfigFile) set(segments []string, value any) error {
	root, err := f.rootNode()
	if err != nil {
		return err
	}

	valueNode, err := encodeValue(value)
	if err != nil {
		return err
	}

	return setInMapping(root, segments, valueNode)
}

// setUnderWhen writes value at the dotted path inside the file's when@<env>
// block, creating the block when it does not exist yet, so the value stays
// scoped to that environment.
func (f *ConfigFile) setUnderWhen(env string, segments []string, value any) error {
	root, err := f.rootNode()
	if err != nil {
		return err
	}

	when := mappingChild(root, whenPrefix+env)
	if when == nil {
		when = newMapping()
		mapPut(root, whenPrefix+env, when)
	}

	valueNode, err := encodeValue(value)
	if err != nil {
		return err
	}

	return setInMapping(when, segments, valueNode)
}

// rootNode returns the document's root mapping, creating an empty document and
// mapping when the file is new or empty.
func (f *ConfigFile) rootNode() (*yaml.Node, error) {
	if f.doc == nil {
		f.doc = &yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{newMapping()},
		}
	}

	if len(f.doc.Content) == 0 {
		f.doc.Content = append(f.doc.Content, newMapping())
	}

	root := f.doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: root node is not a mapping", f.Path)
	}

	return root, nil
}

// save serialises the document back to disk, creating parent directories when
// the file is new.
func (f *ConfigFile) save() error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(4)

	if err := encoder.Encode(f.doc); err != nil {
		return err
	}

	if err := encoder.Close(); err != nil {
		return err
	}

	return os.WriteFile(f.Path, buf.Bytes(), 0o644)
}

// setInMapping descends mapping along segments, creating intermediate mappings
// when absent, and assigns valueNode at the leaf.
func setInMapping(mapping *yaml.Node, segments []string, valueNode *yaml.Node) error {
	key := segments[0]

	if len(segments) == 1 {
		if existing := mappingValueNode(mapping, key); existing != nil {
			// Preserve any line comment attached to the existing value.
			valueNode.LineComment = existing.LineComment
			*existing = *valueNode
		} else {
			mapPut(mapping, key, valueNode)
		}

		return nil
	}

	child := mappingChild(mapping, key)
	if child == nil {
		child = newMapping()
		mapPut(mapping, key, child)
	} else if child.Kind != yaml.MappingNode {
		// Replace a scalar/sequence leaf with a mapping so we can descend.
		*child = *newMapping()
	}

	return setInMapping(child, segments[1:], valueNode)
}

// mappingDepth returns the number of leading segments resolvable inside mapping.
func mappingDepth(mapping *yaml.Node, segments []string) int {
	depth := 0
	current := mapping

	for _, segment := range segments {
		if current == nil || current.Kind != yaml.MappingNode {
			break
		}

		next := mappingValueNode(current, segment)
		if next == nil {
			break
		}

		depth++
		current = next
	}

	return depth
}

// mappingChild returns the value node for key when it is itself a mapping.
func mappingChild(mapping *yaml.Node, key string) *yaml.Node {
	node := mappingValueNode(mapping, key)
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	return node
}

// mappingValueNode returns the value node associated with key in mapping, or nil
// when the key is absent.
func mappingValueNode(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}

	return nil
}

// encodeValue turns a Go value into a YAML node. A *yaml.Node is used verbatim
// so callers can pass pre-built nodes.
func encodeValue(value any) (*yaml.Node, error) {
	if node, ok := value.(*yaml.Node); ok {
		return node, nil
	}

	node := &yaml.Node{}
	if err := node.Encode(value); err != nil {
		return nil, err
	}

	return node, nil
}

// splitPath splits a dotted config path into its segments, ignoring empty ones
// so leading/trailing/duplicate dots do not produce blank keys.
func splitPath(path string) []string {
	raw := strings.Split(path, ".")
	segments := make([]string, 0, len(raw))

	for _, segment := range raw {
		if segment != "" {
			segments = append(segments, segment)
		}
	}

	return segments
}
