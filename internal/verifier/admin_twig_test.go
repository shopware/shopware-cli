package verifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

// Not parallel: the Twig parser swaps a package-level indent config in internal/html.
func TestAdminTwigLinterAllowsLegacyComponents(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "index.html.twig"), `{% block content %}<sw-button>Save</sw-button>{% endblock %}`)

	check := NewCheck()
	cfg := ToolConfig{RootDir: dir, AdminDirectories: []string{dir}, MinShopwareVersion: "6.7.0.0"}

	require.NoError(t, AdminTwigLinter{}.Check(t.Context(), check, cfg))

	assert.Empty(t, check.GetResults())
	require.NoError(t, AdminTwigLinter{}.Fix(t.Context(), cfg))
	content, err := os.ReadFile(filepath.Join(dir, "index.html.twig"))
	require.NoError(t, err)
	assert.Equal(t, `{% block content %}<sw-button>Save</sw-button>{% endblock %}`, string(content))
}
