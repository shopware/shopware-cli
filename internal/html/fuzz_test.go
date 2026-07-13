package html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzLexer feeds random bytes through the lexer and checks that it never
// panics, never enters an infinite loop, and always emits a final EOF token.
// Seeded with the input halves of every testdata fixture.
func FuzzLexer(f *testing.F) {
	files, err := os.ReadDir("testdata")
	if err == nil {
		for _, fi := range files {
			if fi.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join("testdata", fi.Name()))
			if err != nil {
				continue
			}
			parts := strings.SplitN(string(data), "-----", 2)
			f.Add(strings.Trim(parts[0], "\n"))
		}
	}
	// A few hand-picked seeds covering known edge cases.
	f.Add("{% if x %}a{% else %}b{% endif %}")
	f.Add("{{ x|filter({a: 1}) }}")
	f.Add(`{% set x = "a%}b" %}`)
	f.Add(`<a {% if x %}href="y"{% endif %}>z</a>`)
	f.Add("{# unterminated")
	f.Add("{%- if x -%}")
	f.Add("<<<>>>")

	f.Fuzz(func(t *testing.T, input string) {
		// Cap input size so a degenerate fuzzer doesn't OOM.
		if len(input) > 1<<16 {
			t.Skip()
		}
		lex := newLexer(input)
		toks, err := lex.lex()
		// Lex errors are fine; panics and infinite loops are not.
		if err != nil {
			return
		}
		if len(toks) == 0 {
			t.Fatal("lexer returned no tokens")
		}
		if toks[len(toks)-1].Type != tokEOF {
			t.Fatal("lexer did not emit final EOF")
		}
	})
}

// FuzzParser feeds random bytes through NewParser. The parser must either
// return an error or a valid NodeList — never panic or hang.
func FuzzParser(f *testing.F) {
	files, err := os.ReadDir("testdata")
	if err == nil {
		for _, fi := range files {
			if fi.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join("testdata", fi.Name()))
			if err != nil {
				continue
			}
			parts := strings.SplitN(string(data), "-----", 2)
			f.Add(strings.Trim(parts[0], "\n"))
		}
	}

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 1<<16 {
			t.Skip()
		}
		nodes, err := NewParser(input)
		if err != nil {
			return
		}
		// If parse succeeded, Dump must not panic either.
		_ = nodes.Dump(0)
	})
}
