// Package html parses and formats Twig templates that contain HTML.
//
// # Architecture
//
// The package is split into four layers, each in its own file(s):
//
//  1. Position tracking — pos.go
//     A Pos is a byte offset + 1-based line + 1-based byte column. A
//     posTracker advances incrementally so line/column are O(1) per byte
//     scanned, instead of O(n) per node.
//
//  2. Lexer — tokens.go, lexer.go
//     Single-pass tokenizer that recognizes HTML element/attribute tokens,
//     {% statement %}, {{ expression }}, and {# comment #} tokens, plus the
//     whitespace-trim modifiers ({%- -%} etc.). The close-delimiter scan
//     is string-literal and bracket-depth aware so {% set x = "a%}b" %}
//     parses correctly. Twig identifiers are scanned with proper word
//     boundaries (no more {% iff %} matching `if`).
//
//  3. Parser — parser.go, plus per-tag files
//     parseDocument runs the lexer and walks the token stream.
//     parseNodesUntil collects child nodes until a stop condition fires
//     (EOF, the parent tag's EndTag, one of its Followers, or the matching
//     HTML element close tag). Two RawNode chunking contexts (top-level
//     and element-children) keep formatter output stable across passes.
//
//  4. AST + formatter — ast.go and format.go
//     ast.go holds the node type definitions; format.go holds every
//     Dump(int) string method plus IndentConfig. A package-level
//     indentConfig is mutated via ConfiguredNodeList.Dump for back-compat;
//     callers concurrently dumping with different configurations should
//     serialize.
//
// # Adding a Twig tag
//
// Tag dispatch goes through a registry in tags.go. For most tags the
// public RegisterStandaloneTag / RegisterBlockTag helpers are enough:
//
//	func init() {
//	    html.RegisterStandaloneTag("sw_icon")            // {% sw_icon 'foo' %}
//	    html.RegisterBlockTag("trans", "endtrans")       // {% trans %}...{% endtrans %}
//	    html.RegisterBlockTag("if", "endif", "elseif", "else")
//	}
//
// Call them from your own package's init(). They panic on duplicate
// registration so name collisions surface at startup.
//
// Tags with bespoke parsing logic (block, if, parent, set, verbatim, twig
// comment) live in dedicated tag_*.go files and call the lower-level
// registerTag with a custom TagSpec.Parse function.
//
// # Unregistered tags
//
// Unknown tags are folded into the surrounding RawNode (via
// parser.appendRawTokens) so the parser is forward-compatible with future
// Twig syntax — an unknown {% something %} round-trips through Dump as-is.
// This works perfectly for standalone tags. For block tags with a body,
// registration is required: without it the body's contents leak into the
// outer scope and the end tag becomes orphan raw text.
//
// # Public API and back-compat
//
// The public surface is small: NewParser, NewAdminParser, NewStorefrontParser,
// the AST node types (ElementNode, Attribute, RawNode, ...), TraverseNode,
// and IndentConfig. All of it is exercised by callers in internal/verifier/
// and must stay stable. The fixture suite in testdata/ pins formatter output;
// any change must keep all fixtures byte-identical.
//
// # Smoke test
//
// internal/html/smoke_test.go walks a checked-out shopware/storefront tree
// (set HTML_SMOKE_CORPUS=/path/to/storefront) and parses every .twig file.
// It is opt-in so CI is not bound to an external clone.
package html
