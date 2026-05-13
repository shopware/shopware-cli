package html

// Registration of straightforward Twig tags that don't need a dedicated file.
//
// Block tags (have a body and an EndTag):
func init() {
	registerTag(TagSpec{Name: "for", EndTag: "endfor", Followers: []string{"else"}, Parse: makeBlockTagParser("for", "endfor", []string{"else"})})
	registerTag(TagSpec{Name: "embed", EndTag: "endembed", Parse: makeBlockTagParser("embed", "endembed", nil)})
	registerTag(TagSpec{Name: "macro", EndTag: "endmacro", Parse: makeBlockTagParser("macro", "endmacro", nil)})
	registerTag(TagSpec{Name: "apply", EndTag: "endapply", Parse: makeBlockTagParser("apply", "endapply", nil)})
	registerTag(TagSpec{Name: "with", EndTag: "endwith", Parse: makeBlockTagParser("with", "endwith", nil)})

	// Standalone tags (no body, no closing tag):
	registerTag(TagSpec{Name: "include", Parse: makeStandaloneTagParser("include")})
	registerTag(TagSpec{Name: "sw_include", Parse: makeStandaloneTagParser("sw_include")})
	registerTag(TagSpec{Name: "extends", Parse: makeStandaloneTagParser("extends")})
	registerTag(TagSpec{Name: "sw_extends", Parse: makeStandaloneTagParser("sw_extends")})
	registerTag(TagSpec{Name: "import", Parse: makeStandaloneTagParser("import")})
	registerTag(TagSpec{Name: "from", Parse: makeStandaloneTagParser("from")})
	registerTag(TagSpec{Name: "use", Parse: makeStandaloneTagParser("use")})
	registerTag(TagSpec{Name: "do", Parse: makeStandaloneTagParser("do")})
	registerTag(TagSpec{Name: "deprecated", Parse: makeStandaloneTagParser("deprecated")})
	registerTag(TagSpec{Name: "flush", Parse: makeStandaloneTagParser("flush")})
}
