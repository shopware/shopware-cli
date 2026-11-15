package git

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	attributesFile        = ".gitattributes"
	exportIgnoreAttribute = "export-ignore"
	macroPrefix           = "[attr]"
)

type exportIgnoreRule struct {
	pattern string
	// anchored patterns contain a slash and match the path relative to their .gitattributes file;
	// the others match the file name at any depth below it.
	anchored bool
	dirOnly  bool
	ignored  bool
}

// ExportIgnore answers whether a path of a working copy carries the export-ignore attribute,
// following the rules "git archive" applies: every .gitattributes file from the root down to
// the folder of the path is read, the deepest file wins, and within a file the last matching
// line wins. $GIT_DIR/info/attributes and the global attributes file are not read.
type ExportIgnore struct {
	root   string
	macros map[string]bool
	rules  map[string][]exportIgnoreRule
}

// NewExportIgnore creates a matcher for the working copy at root. Attribute files are read on demand.
func NewExportIgnore(root string) *ExportIgnore {
	return &ExportIgnore{
		root:  root,
		rules: make(map[string][]exportIgnoreRule),
	}
}

// Match reports whether relPath, relative to the root, is export-ignored.
// A matching folder excludes everything below it, so callers must not descend into it.
func (e *ExportIgnore) Match(relPath string, isDir bool) (bool, error) {
	relPath = path.Clean(filepath.ToSlash(relPath))
	if relPath == "." || relPath == "" {
		return false, nil
	}

	// Macros can only be defined in the top-level file, so it has to be read first.
	if _, err := e.rulesOf("."); err != nil {
		return false, err
	}

	for dir := path.Dir(relPath); ; dir = path.Dir(dir) {
		rules, err := e.rulesOf(dir)
		if err != nil {
			return false, err
		}

		sub := relPath
		if dir != "." {
			sub = strings.TrimPrefix(relPath, dir+"/")
		}

		for i := len(rules) - 1; i >= 0; i-- {
			if rules[i].matches(sub, isDir) {
				return rules[i].ignored, nil
			}
		}

		if dir == "." {
			return false, nil
		}
	}
}

func (r exportIgnoreRule) matches(relPath string, isDir bool) bool {
	if r.dirOnly && !isDir {
		return false
	}

	if !r.anchored {
		ok, _ := path.Match(r.pattern, path.Base(relPath))
		return ok
	}

	return matchSegments(strings.Split(r.pattern, "/"), strings.Split(relPath, "/"))
}

// matchSegments matches a slash separated pattern where "**" spans any number of folders.
func matchSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}

	if pattern[0] == "**" {
		// A trailing "**" matches everything inside, but not the folder itself.
		if len(pattern) == 1 {
			return len(name) > 0
		}

		for i := range len(name) + 1 {
			if matchSegments(pattern[1:], name[i:]) {
				return true
			}
		}

		return false
	}

	if len(name) == 0 {
		return false
	}

	if ok, _ := path.Match(pattern[0], name[0]); !ok {
		return false
	}

	return matchSegments(pattern[1:], name[1:])
}

func (e *ExportIgnore) rulesOf(dir string) ([]exportIgnoreRule, error) {
	if rules, ok := e.rules[dir]; ok {
		return rules, nil
	}

	rules, err := e.readRules(dir)
	if err != nil {
		return nil, err
	}

	e.rules[dir] = rules

	return rules, nil
}

func (e *ExportIgnore) readRules(dir string) ([]exportIgnoreRule, error) {
	file := filepath.Join(e.root, filepath.FromSlash(dir), attributesFile)

	// Git ignores a .gitattributes file that is a symlink, and so does this.
	info, err := os.Lstat(file)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}

		return nil, fmt.Errorf("cannot read %s: %w", file, err)
	}

	if !info.Mode().IsRegular() {
		return nil, nil
	}

	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", file, err)
	}

	lines := attributeLines(content)

	if dir == "." {
		e.macros = make(map[string]bool)

		for _, line := range lines {
			name, isMacro := strings.CutPrefix(line.pattern, macroPrefix)
			if !isMacro {
				continue
			}

			// "[attr]name" and "[attr] name" are both valid.
			attributes := line.attributes
			if name == "" && len(attributes) > 0 {
				name, attributes = attributes[0], attributes[1:]
			}

			if ignored, ok := e.exportIgnoreState(attributes); ok && name != "" {
				e.macros[name] = ignored
			}
		}
	}

	var rules []exportIgnoreRule

	for _, line := range lines {
		// Negative patterns are forbidden in attribute files, git skips them.
		if strings.HasPrefix(line.pattern, macroPrefix) || strings.HasPrefix(line.pattern, "!") {
			continue
		}

		ignored, ok := e.exportIgnoreState(line.attributes)
		if !ok {
			continue
		}

		pattern := line.pattern
		dirOnly := strings.HasSuffix(pattern, "/")
		pattern = strings.TrimSuffix(pattern, "/")
		anchored := strings.Contains(pattern, "/")
		pattern = strings.TrimPrefix(pattern, "/")

		if pattern == "" {
			continue
		}

		rules = append(rules, exportIgnoreRule{
			// Git negates a character class with "!", path.Match only knows "^".
			pattern:  strings.ReplaceAll(pattern, "[!", "[^"),
			anchored: anchored,
			dirOnly:  dirOnly,
			ignored:  ignored,
		})
	}

	return rules, nil
}

// exportIgnoreState returns what a list of attributes says about export-ignore.
// The second value is false when the list does not mention it at all.
func (e *ExportIgnore) exportIgnoreState(attributes []string) (ignored bool, mentioned bool) {
	for _, attribute := range attributes {
		switch {
		case attribute == exportIgnoreAttribute:
			ignored, mentioned = true, true
		case attribute == "-"+exportIgnoreAttribute,
			attribute == "!"+exportIgnoreAttribute,
			// git archive only honours the plain attribute, not "export-ignore=value".
			strings.HasPrefix(attribute, exportIgnoreAttribute+"="):
			ignored, mentioned = false, true
		default:
			if macro, ok := e.macros[attribute]; ok {
				ignored, mentioned = macro, true
			}
		}
	}

	return ignored, mentioned
}

type attributeLine struct {
	pattern    string
	attributes []string
}

func attributeLines(content []byte) []attributeLine {
	var lines []attributeLine

	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		pattern, rest := splitAttributePattern(line)
		if pattern == "" {
			continue
		}

		lines = append(lines, attributeLine{pattern: pattern, attributes: strings.Fields(rest)})
	}

	return lines
}

// splitAttributePattern separates the pattern from its attributes. A pattern containing
// whitespace is written in double quotes with C-style escapes.
func splitAttributePattern(line string) (string, string) {
	if strings.HasPrefix(line, `"`) {
		if quoted, err := strconv.QuotedPrefix(line); err == nil {
			if pattern, err := strconv.Unquote(quoted); err == nil {
				return pattern, line[len(quoted):]
			}
		}
	}

	pattern, rest, _ := strings.Cut(line, " ")
	if before, after, found := strings.Cut(pattern, "\t"); found {
		return before, after + " " + rest
	}

	return pattern, rest
}
