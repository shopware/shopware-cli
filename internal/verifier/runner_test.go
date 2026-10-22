package verifier

import (
	"bytes"
	"context"
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
	published := false
	results := RunTools(t.Context(), selected, skipped, ToolConfig{
		RootDir:           root,
		ValidationIgnores: []validation.ToolConfigIgnore{{Identifier: "ignored"}},
	}, func(tool Tool, execution *ToolExecution) ToolResult {
		switch tool.Name() {
		case "failed":
			execution.Fail(&ToolError{Err: failure, Stdout: "partial", Stderr: "diagnostic"})
			execution.AddFindings([]validation.CheckResult{
				{Path: filepath.Join(root, "src/file.php"), Severity: validation.SeverityError, Identifier: "kept"},
				{Severity: validation.SeverityError, Identifier: "ignored"},
			}...)
			assert.True(t, published, "findings must reach the callback before the tool returns")
			execution.AddFindings(validation.CheckResult{Identifier: "ignored"})
			return ToolResult{}
		case "inapplicable":
			return ToolResult{SkipReason: "no sources"}
		default:
			execution.AddFindings(validation.CheckResult{Identifier: "ignored"})
			return ToolResult{Note: "finished"}
		}
	}, func(name string, findings []validation.CheckResult) {
		assert.Equal(t, "failed", name)
		assert.Len(t, findings, 1)
		published = true
	})
	require.Len(t, results, 4)
	assert.Equal(t, "completed", results[0].Name)
	assert.Equal(t, "completed", results[0].Status())
	assert.False(t, results[0].HasFindings)
	assert.Equal(t, "excluded", results[1].Name)
	assert.NotEmpty(t, results[1].SkipReason)
	assert.Equal(t, "failed", results[2].Name)
	assert.True(t, results[2].HasFindings)
	assert.ErrorIs(t, results[2].Errors[0], failure)
	assert.Equal(t, "partial", results[2].Errors[0].Stdout)
	assert.Equal(t, "diagnostic", results[2].Errors[0].Stderr)
	require.Len(t, results[2].Findings, 1)
	assert.Equal(t, "src/file.php", results[2].Findings[0].Path)
	assert.Equal(t, "inapplicable", results[3].Name)
	assert.Equal(t, "skipped", results[3].Status())
}

func TestRunToolsFinalizesAfterCleanup(t *testing.T) {
	var output bytes.Buffer
	failure := errors.New("cannot clean up temporary files")
	results := RunTools(t.Context(), ToolList[Tool]{testTool{name: "test"}}, nil, ToolConfig{
		Progress: &Progress{output: &output},
	}, func(_ Tool, execution *ToolExecution) ToolResult {
		defer execution.Fail(&ToolError{Err: failure})
		execution.AddFindings(validation.CheckResult{Identifier: "kept"})
		return ToolResult{Note: "partial work"}
	})
	require.Len(t, results, 1)
	assert.True(t, results[0].HasFindings)
	assert.Equal(t, "partial work", results[0].Note)
	require.Len(t, results[0].Errors, 1)
	assert.ErrorIs(t, results[0].Errors[0], failure)
	assert.Contains(t, output.String(), "Failed test")
	assert.NotContains(t, output.String(), "Finished test")
}

func TestRunToolsReportsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	results := RunTools(ctx, ToolList[Tool]{testTool{name: "test"}}, nil, ToolConfig{}, func(_ Tool, _ *ToolExecution) ToolResult {
		cancel()
		return ToolResult{SkipReason: "no sources"}
	})
	require.Len(t, results, 1)
	assert.Equal(t, "failed", results[0].Status())
	assert.Empty(t, results[0].SkipReason)
	require.Len(t, results[0].Errors, 1)
	assert.ErrorIs(t, results[0].Errors[0], context.Canceled)
}
