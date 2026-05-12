package html

import "sync"

// TagSpec describes how to parse a single Twig statement tag (e.g. "block",
// "if", "for", "include"). The registry is the entry point for adding new
// tag support — see tag_block.go, tag_if.go etc.
//
// Lifecycle:
//   - Parse is called when the parser sees `{% Name ... %}`.
//   - openTok is the Twig statement OPEN token; the parser has not yet
//     consumed the identifier or body. Parse implementations should
//     advance the cursor past `%}` (and past any EndTag closer).
//
// Followers and EndTag are advisory: they tell parseTagBody when to stop
// collecting children. A block tag is "open" if either EndTag is set or
// the body parser is invoked.
type TagSpec struct {
	// Name is the identifier after `{%` (e.g. "block", "if").
	Name string
	// EndTag is the closing tag, e.g. "endblock" for a block tag.
	// Empty string for tags with no body ("set" with `=`, "include", "extends").
	EndTag string
	// Followers are sibling tags that may appear inside the body without
	// closing it (e.g. "elseif", "else" inside "if").
	Followers []string
	// Parse turns the tag into an AST node. It is called with the cursor
	// on the OPEN token of the tag.
	Parse TagParseFunc
}

// TagParseFunc is the per-tag parser hook.
type TagParseFunc func(p *parser, openTok token) (Node, error)

var (
	tagRegistryMu sync.RWMutex
	tagRegistry   = map[string]TagSpec{}
)

// registerTag adds a tag handler to the registry. Called from init() in each
// tag_*.go file. Panics on duplicate registration to surface bugs early.
func registerTag(spec TagSpec) {
	if spec.Name == "" {
		panic("html: tag spec must have a non-empty Name")
	}
	if spec.Parse == nil {
		panic("html: tag spec " + spec.Name + " must have a Parse function")
	}
	tagRegistryMu.Lock()
	defer tagRegistryMu.Unlock()
	if _, dup := tagRegistry[spec.Name]; dup {
		panic("html: duplicate tag registration: " + spec.Name)
	}
	tagRegistry[spec.Name] = spec
}

// lookupTag returns the TagSpec for name, or nil if unregistered.
func lookupTag(name string) *TagSpec {
	tagRegistryMu.RLock()
	defer tagRegistryMu.RUnlock()
	if spec, ok := tagRegistry[name]; ok {
		return &spec
	}
	return nil
}

// isFollower reports whether name is in followers.
func isFollower(name string, followers []string) bool {
	for _, f := range followers {
		if f == name {
			return true
		}
	}
	return false
}
