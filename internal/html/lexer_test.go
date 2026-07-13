package html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLexerNoErrorOnFixtures verifies the lexer can scan every fixture input
// without erroring and reaches end-of-input.
func TestLexerNoErrorOnFixtures(t *testing.T) {
	files, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		t.Run(f.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", f.Name()))
			if err != nil {
				t.Fatal(err)
			}
			parts := strings.SplitN(string(data), "-----", 2)
			input := strings.Trim(parts[0], "\n")
			lex := newLexer(input)
			toks, err := lex.lex()
			assert.NoError(t, err)
			assert.NotEmpty(t, toks)
			assert.Equal(t, tokEOF, toks[len(toks)-1].Type)
		})
	}
}

func TestLexerStringLiteralAware(t *testing.T) {
	src := `{% set x = "contains %}" %}`
	lex := newLexer(src)
	toks, err := lex.lex()
	assert.NoError(t, err)
	assert.Equal(t, 5, len(toks))
	assert.Equal(t, tokTwigStmtOpen, toks[0].Type)
	assert.Equal(t, tokTwigIdent, toks[1].Type)
	assert.Equal(t, "set", toks[1].Lit(src))
	assert.Equal(t, tokTwigRawExpr, toks[2].Type)
	assert.Contains(t, toks[2].Lit(src), `"contains %}"`)
	assert.Equal(t, tokTwigStmtClose, toks[3].Type)
}

func TestLexerWhitespaceTrim(t *testing.T) {
	src := `{%- if x -%}body{%- endif -%}`
	lex := newLexer(src)
	toks, err := lex.lex()
	assert.NoError(t, err)
	assert.True(t, toks[0].TrimLeft, "{%- should set TrimLeft on open")
	assert.True(t, toks[3].TrimRight, "-%} should set TrimRight on close")
}

func TestLexerWordBoundary(t *testing.T) {
	src := `{% iff foo %}`
	lex := newLexer(src)
	toks, err := lex.lex()
	assert.NoError(t, err)
	assert.Equal(t, tokTwigIdent, toks[1].Type)
	assert.Equal(t, "iff", toks[1].Lit(src), "identifier scan must respect word boundaries")
}
