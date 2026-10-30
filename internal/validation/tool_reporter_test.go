package validation

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolReportOutcomes(t *testing.T) {
	tools := []ToolReport{
		{
			ToolResult: ToolResult{Name: "phpstan", HasFindings: true, Note: "Shopware 6.7.0.0 (lowest)"},
			Findings: []CheckResult{{
				Path: "src/a.php", Line: 2, Message: "Unknown method",
				Severity: SeverityError, Identifier: "phpstan/method",
			}},
		},
		{ToolResult: ToolResult{Name: "eslint", Errors: []ToolError{{
			Err: errors.New("cannot parse output"), Command: []string{"node", "eslint"},
			Dir: "/project", Stdout: "partial output", Stderr: "process diagnostic",
		}}}},
		{ToolResult: ToolResult{Name: "builtin", SkipReason: "excluded by --exclude"}},
	}
	for _, format := range ReporterFormats {
		t.Run(format, func(t *testing.T) {
			var reportErr error
			var stderr string
			stdout := captureOutput(func() {
				stderr = captureStderr(func() { reportErr = ReportTools(tools, format, "Checkers") })
			})
			assert.ErrorIs(t, reportErr, ErrToolReportFailed)
			assert.NotContains(t, stdout+stderr, "No problems found")
			switch format {
			case "json":
				var report map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(stdout), &report))
				assert.Len(t, report, 2)
				assert.Contains(t, report, "results")
				assert.Contains(t, report, "tools")
				var results []CheckResult
				require.NoError(t, json.Unmarshal(report["results"], &results))
				assert.Equal(t, tools[0].Findings, results)
				assert.JSONEq(t, `[
					{
						"name": "builtin", "status": "skipped",
						"reason": "excluded by --exclude", "has_findings": false
					},
					{
						"name": "eslint", "status": "invoked", "has_findings": false,
						"errors": [{
							"message": "cannot parse output", "command": ["node", "eslint"],
							"dir": "/project", "stdout": "partial output", "stderr": "process diagnostic"
						}]
					},
					{
						"name": "phpstan", "status": "invoked",
						"note": "Shopware 6.7.0.0 (lowest)", "has_findings": true
					}
				]`, string(report["tools"]))
				assert.Empty(t, stderr, "JSON already contains execution errors")
			case "gitlab":
				var issues []GitLabCodeQualityIssue
				require.NoError(t, json.Unmarshal([]byte(stdout), &issues))
				require.Len(t, issues, 2)
				assert.Equal(t, "src/a.php", issues[1].Location.Path)
				assert.Contains(t, issues[0].Description, "process diagnostic")
				assert.Contains(t, stderr, "cannot parse output")
				assert.Contains(t, stderr, "process diagnostic")
			case "junit":
				var suite JUnitTestSuite
				require.NoError(t, xml.Unmarshal([]byte(stdout), &suite))
				assert.Equal(t, 4, suite.Tests)
				assert.Equal(t, 2, suite.Errors)
				assert.Equal(t, 1, suite.Skipped)
				require.NotNil(t, suite.TestCase[2].Error)
				assert.Equal(t, "eslint", suite.TestCase[2].Name)
				assert.Contains(t, stderr, "process diagnostic")
			default:
				assert.Contains(t, stdout, "completed")
				assert.Contains(t, stdout, "failed")
				assert.Contains(t, stdout, "skipped")
				assert.Contains(t, stdout, "1 error · Shopware")
				assert.Contains(t, stdout, "partial output")
				assert.Less(t, strings.Index(stdout, "Checkers"), strings.Index(stdout, "Unknown method"))
			}
		})
	}
}

func TestToolReportExitConditions(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		tool   ToolReport
		failed bool
	}{
		{"completed", ToolReport{ToolResult: ToolResult{Name: "test"}}, false},
		{"warning", ToolReport{ToolResult: ToolResult{Name: "test", HasFindings: true}, Findings: []CheckResult{{Severity: SeverityWarning}}}, false},
		{"finding", ToolReport{ToolResult: ToolResult{Name: "test", HasFindings: true}, Findings: []CheckResult{{Severity: SeverityError}}}, true},
		{"execution", ToolReport{ToolResult: ToolResult{Name: "test", Errors: []ToolError{{Err: errors.New("cannot run test")}}}}, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var reportErr error
			stdout := captureOutput(func() {
				reportErr = ReportTools([]ToolReport{scenario.tool}, "summary", "Fixers")
			})
			if scenario.failed {
				assert.ErrorIs(t, reportErr, ErrToolReportFailed)
				assert.NotContains(t, stdout, "No problems found")
			} else {
				assert.NoError(t, reportErr)
			}
		})
	}
}

func TestToolReportIncludesAllFailuresAndPartialFindings(t *testing.T) {
	tools := []ToolReport{{
		ToolResult: ToolResult{Name: "eslint", HasFindings: true, Errors: []ToolError{
			{Err: errors.New("cannot load configuration"), Dir: "/first", Stdout: "first output"},
			{Err: errors.New("cannot parse report"), Dir: "/second", Stderr: "second output"},
		}},
		Findings: []CheckResult{{Path: "file.js", Identifier: "eslint/test", Message: "source warning", Severity: SeverityWarning}},
	}}
	for _, format := range ReporterFormats {
		t.Run(format, func(t *testing.T) {
			var stderr string
			stdout := captureOutput(func() {
				stderr = captureStderr(func() {
					assert.ErrorIs(t, ReportTools(tools, format, "Checkers"), ErrToolReportFailed)
				})
			})
			for _, detail := range []string{"cannot load configuration", "cannot parse report", "/first", "/second", "first output", "second output", "source warning"} {
				assert.Contains(t, stdout, detail)
			}
			if format == "json" {
				assert.Contains(t, stdout, `"findings_incomplete": true`)
			} else {
				assert.Contains(t, stdout+stderr, "may be incomplete")
			}
		})
	}
}

func TestGitHubToolFailureEscaping(t *testing.T) {
	tools := []ToolReport{{ToolResult: ToolResult{Name: "eslint", Errors: []ToolError{{
		Err: errors.New("bad%0A\n::warning::injected"), Stdout: "::error::raw output",
	}}}}}
	stdout := captureOutput(func() { assert.ErrorIs(t, ReportTools(tools, "github", "Checkers"), ErrToolReportFailed) })
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.True(t, strings.HasPrefix(lines[0], "::stop-commands::"))
	assert.Equal(t, "::error title=eslint::bad%250A%0A::warning::injected", lines[len(lines)-1])
}
