package verifier

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/validation"
)

func TestRunToolsRetainsIndependentResults(t *testing.T) {
	root := t.TempDir()
	failure := errors.New("cannot finish analysis")
	all := ToolList[Tool]{testTool{name: "failed"}, testTool{name: "completed"}, testTool{name: "excluded"}, testTool{name: "inapplicable"}}
	selected, skipped, err := SelectTools(all, "", "excluded", "checkers")
	require.NoError(t, err)
	results := RunTools(selected, skipped, ToolConfig{
		RootDir:           root,
		ValidationIgnores: []validation.ToolConfigIgnore{{Identifier: "ignored"}},
	}, func(tool Tool) ToolResult {
		switch tool.Name() {
		case "failed":
			return ToolResult{Err: failure, Stdout: "partial", Stderr: "diagnostic", Findings: []validation.CheckResult{
				{Path: filepath.Join(root, "src/file.php"), Severity: validation.SeverityError, Identifier: "kept"},
				{Severity: validation.SeverityError, Identifier: "ignored"},
			}}
		case "inapplicable":
			return ToolResult{SkipReason: "no sources"}
		default:
			return ToolResult{Note: "finished"}
		}
	})
	require.Len(t, results, 4)
	assert.Equal(t, "completed", results[0].Name)
	assert.Equal(t, "completed", results[0].Status())
	assert.Equal(t, "excluded", results[1].Name)
	assert.NotEmpty(t, results[1].SkipReason)
	assert.Equal(t, "failed", results[2].Name)
	assert.ErrorIs(t, results[2].Err, failure)
	assert.Equal(t, "partial", results[2].Stdout)
	assert.Equal(t, "diagnostic", results[2].Stderr)
	require.Len(t, results[2].Findings, 1)
	assert.Equal(t, "src/file.php", results[2].Findings[0].Path)
	assert.Equal(t, "inapplicable", results[3].Name)
	assert.Equal(t, "skipped", results[3].Status())
}
