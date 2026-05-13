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
// # Adding a new Twig tag
//
// Twig tag dispatch goes through a registry — tags.go defines TagSpec and
// the registerTag function. Each tag is one file:
//
//	internal/html/tag_<name>.go
//	    func init() { registerTag(TagSpec{...}) }
//	    func parse<Name>Tag(p *parser, openTok token) (Node, error) { ... }
//
// See tag_block.go, tag_if.go, tag_parent.go for working examples.
//
//   - TagSpec.EndTag declares the closing tag ("endblock", "endif", ...).
//     Tags with no body (e.g. {% set x = 1 %}, {% include "..." %}) leave
//     EndTag empty.
//   - TagSpec.Followers declares sibling tags that appear inside the body
//     without closing it (e.g. "elseif", "else" inside an "if").
//   - The handler is responsible for advancing past its tokens, parsing
//     the body via parser.parseNodesUntil(...), and consuming the close
//     tag via parser.consumeEndTag(...).
//
// Unrecognized tags are folded into the surrounding RawNode (via
// parser.appendRawTokens) so the parser is forward-compatible with future
// Twig syntax: an unknown {% something %} round-trips through Dump as-is.
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
