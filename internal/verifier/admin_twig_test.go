package verifier

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

// Not parallel: the Twig parser swaps a package-level indent config in internal/html.
func TestAdminTwigLinterReportsRemovedComponents(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "index.html.twig"), `{% block content %}<sw-button>Save</sw-button>{% endblock %}`)

	check := NewCheck()
	cfg := ToolConfig{RootDir: dir, AdminDirectories: []string{dir}, MinShopwareVersion: "6.7.0.0"}

	require.NoError(t, AdminTwigLinter{}.Check(t.Context(), check, cfg))

	results := check.GetResults()
	require.Len(t, results, 1)
	assert.Equal(t, "admintwiglinter/sw-button", results[0].Identifier)
	assert.Equal(t, "index.html.twig", results[0].Path)
}
