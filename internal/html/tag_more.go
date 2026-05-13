package html

// Built-in Twig tags. Tags with their own non-trivial parser (block, if,
// parent, set, verbatim, twig comment) live in dedicated files. Everything
// else funnels through the makeBlockTagParser / makeStandaloneTagParser
// helpers.
//
// Custom project-specific tags should be registered from the caller's own
// init() via RegisterBlockTag / RegisterStandaloneTag — see tags.go.
func init() {
	// Block tags (body + EndTag).
	RegisterBlockTag("for", "endfor", "else")
	RegisterBlockTag("embed", "endembed")
	RegisterBlockTag("macro", "endmacro")
	RegisterBlockTag("apply", "endapply")
	RegisterBlockTag("with", "endwith")

	// Standalone tags (no body, no closing tag). Twig core + Shopware
	// storefront extensions. Unknown standalone tags still round-trip
	// fine via the raw-text fallback; registering them just gives
	// downstream AST consumers a structured node.
	RegisterStandaloneTag("include")
	RegisterStandaloneTag("sw_include")
	RegisterStandaloneTag("extends")
	RegisterStandaloneTag("sw_extends")
	RegisterStandaloneTag("import")
	RegisterStandaloneTag("from")
	RegisterStandaloneTag("use")
	RegisterStandaloneTag("do")
	RegisterStandaloneTag("deprecated")
	RegisterStandaloneTag("flush")
	RegisterStandaloneTag("break")
	RegisterStandaloneTag("return")
	RegisterStandaloneTag("sw_icon")
	RegisterStandaloneTag("sw_thumbnails")
}
