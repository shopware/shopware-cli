package html

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPreContentIsNotReformatted(t *testing.T) {
	t.Parallel()

	input := `<pre>
line1
    indented line2
        deeper
</pre>`

	nodes, err := NewParser(input)
	assert.NoError(t, err)
	assert.Equal(t, input, nodes.Dump(0))
}

func TestPreWithNestedElementsIsPreservedVerbatim(t *testing.T) {
	t.Parallel()

	input := `<pre><code class="language-twig">{{ product.name }}
    {% if condition %}indented{% endif %}
</code></pre>`

	nodes, err := NewParser(input)
	assert.NoError(t, err)
	assert.Equal(t, input, nodes.Dump(0))
}

func TestTextareaContentIsNotReformatted(t *testing.T) {
	t.Parallel()

	input := `<textarea name="notes">
  keep
     this
spacing</textarea>`

	nodes, err := NewParser(input)
	assert.NoError(t, err)
	assert.Equal(t, input, nodes.Dump(0))
}
