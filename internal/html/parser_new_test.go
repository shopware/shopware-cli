package html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewParserMatchesLegacy runs the new lexer-driven parser against every
// fixture and checks that its formatter output matches the legacy parser's.
// While the new parser is still under construction this acts as the
// fixture-driven iteration target.
func TestNewParserMatchesLegacy(t *testing.T) {
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
			parts := strings.SplitN(string(data), "-----", 3)
			input := strings.Trim(parts[0], "\n")

			legacy, err := NewParser(input)
			assert.NoError(t, err, "legacy parser must succeed")
			legacyOut := legacy.Dump(0)

			p := &parser{source: input}
			newNodes, err := p.parseDocument()
			assert.NoError(t, err, "new parser must succeed")
			newOut := newNodes.Dump(0)

			assert.Equal(t, legacyOut, newOut, "new parser output must match legacy")
		})
	}
}
