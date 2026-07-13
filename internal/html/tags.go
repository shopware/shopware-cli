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
	tagRegistryMu sync.Mutex
	tagRegistry   = map[string]*TagSpec{}
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
	specCopy := spec
	tagRegistry[spec.Name] = &specCopy
}

// lookupTag returns the TagSpec for name, or nil if unregistered.
//
// The registry is populated entirely from init() functions in the tag_*.go
// files, which run before any parsing can occur, so it is effectively immutable
// on the read path. Looking up without locking avoids RWMutex overhead on the
// parser hot path; the stored *TagSpec is never mutated after registration.
func lookupTag(name string) *TagSpec {
	if spec, ok := tagRegistry[name]; ok {
		return spec
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

// RegisterStandaloneTag registers a Twig tag that has no body, e.g.
// `{% sw_icon 'foo' %}` or `{% include 'x.twig' %}`. The parser will yield
// a *TwigStandaloneTagNode with the tag's name and verbatim argument body.
//
// Call from an init() in your own package to teach the parser about
// project-specific tags. Standalone tags that are NOT registered still
// round-trip through Dump as raw text — registration just gives downstream
// AST consumers a semantic node to reason about.
//
// Panics if name is empty or already registered.
func RegisterStandaloneTag(name string) {
	registerTag(TagSpec{Name: name, Parse: makeStandaloneTagParser(name)})
}

// RegisterBlockTag registers a Twig tag that wraps a body, e.g.
// `{% trans %}...{% endtrans %}`. The endTag must match what closes the
// block (typically "end"+name). Optional followers are sibling tags that
// appear inside the body without closing it (e.g. for `{% if %}` the
// followers are `{"elseif", "else"}`); pass none for tags like `{% trans %}`.
//
// Unlike standalone tags, block tags MUST be registered for the parser to
// know where the body ends — without registration the body's contents leak
// into the outer scope and the end tag becomes orphan raw text.
//
// Panics if name is empty or already registered.
func RegisterBlockTag(name, endTag string, followers ...string) {
	registerTag(TagSpec{
		Name:      name,
		EndTag:    endTag,
		Followers: followers,
		Parse:     makeBlockTagParser(name, endTag, followers),
	})
}
