package verifier

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
	"github.com/shopware/shopware-cli/internal/validation"
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

// Not parallel: the Twig parser swaps a package-level indent config in internal/html.
func TestAdminTwigLinterRecordsWhetherRulesApply(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "index.html.twig"), `{% block content %}<sw-button>Save</sw-button>{% endblock %}`)

	cases := []struct {
		baseline string
		run      validation.ToolRun
		findings bool
	}{
		{
			baseline: "6.6.10.21",
			run:      validation.ToolRun{Name: "admin-twig", Status: validation.ToolRunSkipped, Baseline: "6.6.10.21", Note: "no admin Twig rules apply to Shopware 6.6.10.21"},
		},
		{
			baseline: "6.7.0.0",
			run:      validation.ToolRun{Name: "admin-twig", Status: validation.ToolRunRan, Baseline: "6.7.0.0"},
			findings: true,
		},
	}

	for _, tc := range cases {
		check := NewCheck()
		cfg := ToolConfig{RootDir: dir, AdminDirectories: []string{dir}, MinShopwareVersion: tc.baseline}

		require.NoError(t, AdminTwigLinter{}.Check(t.Context(), check, cfg))

		assert.Equal(t, []validation.ToolRun{tc.run}, check.GetToolRuns(), tc.baseline)
		assert.Equal(t, tc.findings, len(check.GetResults()) > 0, tc.baseline)
	}
}
