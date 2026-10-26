package verifier

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"path/filepath"
	"slices"
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
	results := RunTools(t.Context(), selected, skipped, ToolConfig{
		RootDir:           root,
		ValidationIgnores: []validation.ToolConfigIgnore{{Identifier: "ignored"}},
	}, nil, func(tool Tool, execution *ToolExecution) ToolSummary {
		switch tool.Name() {
		case "failed":
			execution.Fail(&ToolError{Err: failure, Stdout: "partial", Stderr: "diagnostic"})
			execution.AddFindings([]validation.CheckResult{
				{Path: filepath.Join(root, "src/file.php"), Severity: validation.SeverityError, Identifier: "kept"},
				{Severity: validation.SeverityError, Identifier: "ignored"},
			}...)
			execution.AddFindings(validation.CheckResult{Identifier: "ignored"})
			return ToolSummary{}
		case "inapplicable":
			return ToolSummary{SkipReason: "no sources"}
		default:
			execution.AddFindings(validation.CheckResult{Identifier: "ignored"})
			return ToolSummary{Note: "finished"}
		}
	})
	require.Len(t, results, 4)
	slices.SortFunc(results, func(a, b validation.ToolReport) int { return cmp.Compare(a.Name, b.Name) })
	for _, result := range results {
		assert.Equal(t, len(result.Findings) > 0, result.HasFindings, result.Name)
	}
	assert.Equal(t, "completed", results[0].Name)
	assert.Equal(t, "completed", results[0].Status())
	assert.Empty(t, results[0].Findings)
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
	results := RunTools(t.Context(), ToolList[Tool]{testTool{name: "test"}}, nil, ToolConfig{}, &Progress{output: &output}, func(_ Tool, execution *ToolExecution) ToolSummary {
		defer execution.Fail(&ToolError{Err: failure})
		execution.AddFindings(validation.CheckResult{Identifier: "kept"})
		return ToolSummary{Note: "partial work"}
	})
	require.Len(t, results, 1)
	assert.Len(t, results[0].Findings, 1)
	assert.True(t, results[0].HasFindings)
	assert.Equal(t, "partial work", results[0].Note)
	require.Len(t, results[0].Errors, 1)
	assert.ErrorIs(t, results[0].Errors[0], failure)
	assert.Contains(t, output.String(), "Failed test")
	assert.NotContains(t, output.String(), "Finished test")
}

func TestRunToolsReportsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	results := RunTools(ctx, ToolList[Tool]{testTool{name: "test"}}, nil, ToolConfig{}, nil, func(_ Tool, _ *ToolExecution) ToolSummary {
		cancel()
		return ToolSummary{SkipReason: "no sources"}
	})
	require.Len(t, results, 1)
	assert.Equal(t, "failed", results[0].Status())
	assert.Empty(t, results[0].SkipReason)
	require.Len(t, results[0].Errors, 1)
	assert.ErrorIs(t, results[0].Errors[0], context.Canceled)
}

func TestRunToolsIgnoresSkipReasonAfterFindingsOrFailures(t *testing.T) {
	results := RunTools(t.Context(), ToolList[Tool]{testTool{name: "findings"}, testTool{name: "ignored"}, testTool{name: "failed"}}, nil, ToolConfig{
		ValidationIgnores: []validation.ToolConfigIgnore{{Identifier: "ignored"}},
	}, nil, func(tool Tool, execution *ToolExecution) ToolSummary {
		switch tool.Name() {
		case "findings":
			execution.AddFindings(validation.CheckResult{Identifier: "kept", Severity: validation.SeverityError})
		case "ignored":
			execution.AddFindings(validation.CheckResult{Identifier: "ignored", Severity: validation.SeverityError})
		case "failed":
			execution.Fail(&ToolError{Err: errors.New("cannot run test")})
		}
		return ToolSummary{SkipReason: "no sources"}
	})
	require.Len(t, results, 3)
	assert.Equal(t, "completed", results[0].Status())
	assert.True(t, results[0].HasFindings)
	assert.Equal(t, "skipped", results[1].Status(), "only ignored findings must not prevent a skip")
	assert.False(t, results[1].HasFindings)
	assert.Equal(t, "failed", results[2].Status())
	assert.Empty(t, results[2].SkipReason)
}
