package html

// Helpers that emit Twig delimiter pairs, honoring whitespace-control
// modifiers. The lexer captures `{%-`, `-%}`, `{{-`, `-}}`, `{#-`, `-#}` on
// open/close tokens; the parser stores those flags on the corresponding AST
// node, and the formatter emits them back through these helpers.
//
// Using these everywhere — rather than hard-coded "{%" / "%}" literals —
// ensures a node that was parsed with trim modifiers round-trips exactly.

func openStmt(t bool) string {
	if t {
		return "{%-"
	}
	return "{%"
}

func closeStmt(t bool) string {
	if t {
		return "-%}"
	}
	return "%}"
}

func openExpr(t bool) string {
	if t {
		return "{{-"
	}
	return "{{"
}

func closeExpr(t bool) string {
	if t {
		return "-}}"
	}
	return "}}"
}

func openComment(t bool) string {
	if t {
		return "{#-"
	}
	return "{#"
}

func closeComment(t bool) string {
	if t {
		return "-#}"
	}
	return "#}"
}
