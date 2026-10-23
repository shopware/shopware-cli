package validation

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolReportOutcomes(t *testing.T) {
	tools := []ToolResult{
		{Name: "phpstan", Note: "Shopware 6.7.0.0 (lowest)", Findings: []CheckResult{{Path: "src/a.php", Line: 2, Message: "Unknown method", Severity: SeverityError, Identifier: "phpstan/method"}}},
		{Name: "eslint", Err: errors.New("cannot parse output"), Stdout: "partial output", Stderr: "process diagnostic"},
		{Name: "builtin", SkipReason: "excluded by --exclude"},
	}
	for _, format := range ReporterFormats {
		t.Run(format, func(t *testing.T) {
			var reportErr error
			var stderr string
			stdout := captureOutput(func() {
				stderr = captureStderr(func() { reportErr = ReportTools(tools, format, "Checkers", false) })
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
				var statuses []map[string]string
				require.NoError(t, json.Unmarshal(report["tools"], &statuses))
				assert.Equal(t, []map[string]string{
					{"name": "builtin", "status": "skipped", "reason": "excluded by --exclude"},
					{"name": "eslint", "status": "invoked", "reason": "cannot parse output"},
					{"name": "phpstan", "status": "invoked", "reason": "Shopware 6.7.0.0 (lowest)"},
				}, statuses)
				assert.NotContains(t, stdout, "partial output")
				assert.Contains(t, stderr, "partial output")
			case "gitlab":
				var issues []GitLabCodeQualityIssue
				require.NoError(t, json.Unmarshal([]byte(stdout), &issues))
				require.Len(t, issues, 1)
				assert.Equal(t, "src/a.php", issues[0].Location.Path)
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
	for _, format := range ReporterFormats {
		for _, scenario := range []struct {
			name   string
			tool   ToolResult
			failed bool
		}{
			{"completed", ToolResult{Name: "test"}, false},
			{"warning", ToolResult{Name: "test", Findings: []CheckResult{{Severity: SeverityWarning}}}, false},
			{"finding", ToolResult{Name: "test", Findings: []CheckResult{{Severity: SeverityError}}}, true},
			{"execution", ToolResult{Name: "test", Err: errors.New("cannot run test")}, true},
			{"skipped", ToolResult{Name: "test", SkipReason: "no sources"}, false},
		} {
			t.Run(format+"/"+scenario.name, func(t *testing.T) {
				var err error
				stdout := captureOutput(func() {
					_ = captureStderr(func() { err = ReportTools([]ToolResult{scenario.tool}, format, "Fixers", false) })
				})
				if scenario.failed {
					assert.ErrorIs(t, err, ErrToolReportFailed)
				} else {
					assert.NoError(t, err)
				}
				if scenario.failed || scenario.tool.SkipReason != "" {
					assert.NotContains(t, stdout, "No problems found")
				}
			})
		}
	}
}

func TestToolReportVerboseOutput(t *testing.T) {
	tools := []ToolResult{{Name: "prettier", Note: "2 files formatted", Stdout: "formatted file list"}}
	for _, verbose := range []bool{false, true} {
		stdout := captureOutput(func() { require.NoError(t, ReportTools(tools, "summary", "Formatters", verbose)) })
		assert.Contains(t, stdout, "Formatters:")
		assert.Contains(t, stdout, "2 files formatted")
		assert.Equal(t, verbose, strings.Contains(stdout, "formatted file list"))
	}
	var stderr string
	stdout := captureOutput(func() {
		stderr = captureStderr(func() { require.NoError(t, ReportTools(tools, "json", "Formatters", true)) })
	})
	assert.True(t, json.Valid([]byte(stdout)))
	assert.NotContains(t, stdout, "formatted file list")
	assert.Contains(t, stderr, "formatted file list")
}

func TestGitHubToolFailureEscaping(t *testing.T) {
	tools := []ToolResult{{Name: "eslint", Err: errors.New("bad%0A\n::warning::injected"), Stdout: "::error::raw output"}}
	stdout := captureOutput(func() { assert.ErrorIs(t, ReportTools(tools, "github", "Checkers", false), ErrToolReportFailed) })
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.True(t, strings.HasPrefix(lines[0], "::stop-commands::"))
	assert.Equal(t, "::error title=eslint::bad%250A%0A::warning::injected", lines[len(lines)-1])
}

type failingReportWriter struct{}

func (failingReportWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestToolReportWriteFailure(t *testing.T) {
	assert.ErrorIs(t, writeToolSummary(failingReportWriter{}, []ToolResult{{Name: "test"}}, "Checkers", false, false), io.ErrClosedPipe)
}
