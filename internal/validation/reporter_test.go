package validation

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportingOutputIsDeterministic(t *testing.T) {
	// Create test results in non-alphabetical order
	testResults := []CheckResult{
		{
			Path:       "z_file.go",
			Line:       5,
			Identifier: "test.rule2",
			Message:    "Second message",
			Severity:   SeverityError,
		},
		{
			Path:       "a_file.go",
			Line:       10,
			Identifier: "test.rule1",
			Message:    "First message",
			Severity:   SeverityWarning,
		},
		{
			Path:       "a_file.go",
			Line:       5,
			Identifier: "test.rule3",
			Message:    "Third message",
			Severity:   SeverityError,
		},
		{
			Path:       "b_file.go",
			Line:       1,
			Identifier: "test.rule1",
			Message:    "Fourth message",
			Severity:   SeverityWarning,
		},
	}

	check := &testCheck{Results: testResults}

	// Test summary report multiple times to ensure deterministic output
	for range 5 {
		output := captureOutput(func() {
			// Ignore the error since we're testing output format, not validation logic
			_ = doSummaryReport(check)
		})

		// Check that files are sorted alphabetically
		lines := strings.Split(output, "\n")
		var fileHeaderLines []string
		for _, line := range lines {
			// File headers are lines that contain .go but not indented (no leading spaces)
			if strings.Contains(line, ".go") && !strings.HasPrefix(line, " ") {
				fileHeaderLines = append(fileHeaderLines, strings.TrimSpace(line))
			}
		}

		assert.Len(t, fileHeaderLines, 3) // 3 files
		assert.Equal(t, "a_file.go", fileHeaderLines[0])
		assert.Equal(t, "b_file.go", fileHeaderLines[1])
		assert.Equal(t, "z_file.go", fileHeaderLines[2])
	}

	// Test GitHub report multiple times to ensure deterministic output
	for range 5 {
		output := captureOutput(func() {
			_ = doGitHubReport(check)
		})

		// The GitHub reporter now prefixes the summary output before the
		// annotations. Extract only the annotation lines for ordering checks.
		var annotationLines []string
		for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
			if strings.HasPrefix(line, "::error") || strings.HasPrefix(line, "::warning") {
				annotationLines = append(annotationLines, line)
			}
		}
		assert.Len(t, annotationLines, 4) // 4 results

		// Check that results are sorted by path first, then by line
		assert.Contains(t, annotationLines[0], "a_file.go")
		assert.Contains(t, annotationLines[0], "line=5")
		assert.Contains(t, annotationLines[1], "a_file.go")
		assert.Contains(t, annotationLines[1], "line=10")
		assert.Contains(t, annotationLines[2], "b_file.go")
		assert.Contains(t, annotationLines[2], "line=1")
		assert.Contains(t, annotationLines[3], "z_file.go")
		assert.Contains(t, annotationLines[3], "line=5")

		// And the summary header should be present
		assert.Contains(t, output, "problems")
	}
}

func TestMarkdownReportIsDeterministic(t *testing.T) {
	testResults := []CheckResult{
		{
			Path:       "z_file.go",
			Line:       5,
			Identifier: "test.rule2",
			Message:    "Second message",
			Severity:   SeverityError,
		},
		{
			Path:       "a_file.go",
			Line:       10,
			Identifier: "test.rule1",
			Message:    "First message",
			Severity:   SeverityWarning,
		},
	}

	check := &testCheck{Results: testResults}

	// Test markdown report multiple times to ensure deterministic output
	for range 5 {
		output := captureOutput(func() {
			_ = doMarkdownReport(check)
		})

		// Check that files are sorted alphabetically
		lines := strings.Split(output, "\n")
		var headerLines []string
		for _, line := range lines {
			if strings.HasPrefix(line, "## ") {
				headerLines = append(headerLines, line)
			}
		}

		assert.Len(t, headerLines, 2) // 2 files
		assert.Contains(t, headerLines[0], "a_file.go")
		assert.Contains(t, headerLines[1], "z_file.go")
	}
}

func TestErrorExistsSummary(t *testing.T) {
	testResults := []CheckResult{
		{
			Path:       "z_file.go",
			Line:       5,
			Identifier: "test.rule2",
			Message:    "Second message",
			Severity:   SeverityError,
		},
		{
			Path:       "a_file.go",
			Line:       10,
			Identifier: "test.rule1",
			Message:    "First message",
			Severity:   SeverityWarning,
		},
	}

	check := &testCheck{Results: testResults}

	assert.Error(t, DoCheckReport(check, "summary"))
}

func TestValidateReporter(t *testing.T) {
	for _, format := range []string{"summary", "json", "github", "gitlab", "junit", "markdown"} {
		assert.NoError(t, ValidateReporter(format))
	}

	assert.EqualError(t, ValidateReporter("yaml"), `invalid reporting format "yaml", allowed values: summary, json, github, gitlab, junit, markdown`)
}

func TestGitLabReport(t *testing.T) {
	testResults := []CheckResult{
		{
			Path:       "src/index.js",
			Line:       42,
			Identifier: "no-unused-vars",
			Message:    "'unused' is assigned a value but never used.",
			Severity:   SeverityWarning,
		},
		{
			Path:       "src/utils.js",
			Line:       15,
			Identifier: "syntax-error",
			Message:    "Missing semicolon",
			Severity:   SeverityError,
		},
	}

	check := &testCheck{Results: testResults}

	output := captureOutput(func() {
		err := doGitLabReport(check)
		assert.NoError(t, err)
	})

	// Parse the JSON output
	var issues []GitLabCodeQualityIssue
	err := json.Unmarshal([]byte(output), &issues)
	assert.NoError(t, err)
	assert.Len(t, issues, 2)

	// Check first issue (should be sorted by path then line)
	issue1 := issues[0]
	assert.Equal(t, "'unused' is assigned a value but never used.", issue1.Description)
	assert.Equal(t, "no-unused-vars", issue1.CheckName)
	assert.Equal(t, "minor", issue1.Severity) // Warning maps to minor
	assert.Equal(t, "src/index.js", issue1.Location.Path)
	assert.Equal(t, 42, issue1.Location.Lines.Begin)
	assert.NotEmpty(t, issue1.Fingerprint) // Should have fingerprint

	// Check second issue
	issue2 := issues[1]
	assert.Equal(t, "Missing semicolon", issue2.Description)
	assert.Equal(t, "syntax-error", issue2.CheckName)
	assert.Equal(t, "major", issue2.Severity) // Error maps to major
	assert.Equal(t, "src/utils.js", issue2.Location.Path)
	assert.Equal(t, 15, issue2.Location.Lines.Begin)
	assert.NotEmpty(t, issue2.Fingerprint)

	// Ensure fingerprints are different
	assert.NotEqual(t, issue1.Fingerprint, issue2.Fingerprint)
}

func TestGitLabReportIsDeterministic(t *testing.T) {
	testResults := []CheckResult{
		{
			Path:       "z_file.go",
			Line:       5,
			Identifier: "test.rule2",
			Message:    "Second message",
			Severity:   SeverityError,
		},
		{
			Path:       "a_file.go",
			Line:       10,
			Identifier: "test.rule1",
			Message:    "First message",
			Severity:   SeverityWarning,
		},
	}

	check := &testCheck{Results: testResults}

	// Test GitLab report multiple times to ensure deterministic output
	var previousOutput string
	for i := range 5 {
		output := captureOutput(func() {
			_ = doGitLabReport(check)
		})

		if i > 0 {
			assert.Equal(t, previousOutput, output, "GitLab report output should be deterministic")
		}
		previousOutput = output

		// Parse and verify the issues are sorted correctly
		var issues []GitLabCodeQualityIssue
		err := json.Unmarshal([]byte(output), &issues)
		assert.NoError(t, err)
		assert.Len(t, issues, 2)

		// Check sorting: should be sorted by path first, then by line
		assert.Equal(t, "a_file.go", issues[0].Location.Path)
		assert.Equal(t, 10, issues[0].Location.Lines.Begin)
		assert.Equal(t, "z_file.go", issues[1].Location.Path)
		assert.Equal(t, 5, issues[1].Location.Lines.Begin)
	}
}

// testCheck is a simple implementation of Check interface for testing
type testCheck struct {
	Results  []CheckResult
	Target   *Target
	ToolRuns []ToolRun
}

func (c *testCheck) GetTarget() *Target {
	return c.Target
}

func (c *testCheck) GetToolRuns() []ToolRun {
	return c.ToolRuns
}

func (c *testCheck) AddResult(result CheckResult) {
	c.Results = append(c.Results, result)
}

func (c *testCheck) GetResults() []CheckResult {
	return c.Results
}

func (c *testCheck) HasErrors() bool {
	for _, r := range c.Results {
		if r.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (c *testCheck) RemoveByIdentifier(ignores []ToolConfigIgnore) Check {
	// Simple implementation for testing
	return c
}

func TestSummaryReportWithTip(t *testing.T) {
	testResults := []CheckResult{
		{
			Path:       "src/Service.php",
			Line:       10,
			Identifier: "phpstan/missingType",
			Message:    "Method has no return type",
			Severity:   SeverityError,
			Tip:        "Add a return type declaration",
		},
		{
			Path:       "src/Service.php",
			Line:       20,
			Identifier: "phpstan/other",
			Message:    "Some other error",
			Severity:   SeverityError,
		},
	}

	check := &testCheck{Results: testResults}

	output := captureOutput(func() {
		_ = doSummaryReport(check)
	})

	assert.Contains(t, output, "Method has no return type")
	assert.Contains(t, output, "Tip: Add a return type declaration")
	assert.Contains(t, output, "Some other error")
}

func TestGitHubReportNeutralizesSummaryCommands(t *testing.T) {
	// A malicious path containing a workflow command must not be executed
	// by the runner when emitted via the summary section.
	testResults := []CheckResult{
		{
			Path:       "::add-mask::secret",
			Line:       1,
			Identifier: "test.rule",
			Message:    "boom",
			Severity:   SeverityError,
		},
	}

	check := &testCheck{Results: testResults}

	output := captureOutput(func() {
		_ = doGitHubReport(check)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.True(t, strings.HasPrefix(lines[0], "::stop-commands::"), "summary must be wrapped in ::stop-commands::")

	token := strings.TrimPrefix(lines[0], "::stop-commands::")
	assert.NotEmpty(t, token)

	// The matching resume marker must appear before any annotation lines.
	resume := "::" + token + "::"
	resumeIdx := -1
	for i, line := range lines {
		if line == resume {
			resumeIdx = i
			break
		}
	}
	assert.GreaterOrEqual(t, resumeIdx, 1, "resume marker must follow the stop marker")

	// Annotations should appear only after the resume marker.
	for i, line := range lines {
		if strings.HasPrefix(line, "::error") || strings.HasPrefix(line, "::warning") {
			assert.Greater(t, i, resumeIdx, "annotations must come after resume marker")
		}
	}
}

func TestGitHubReportWithTip(t *testing.T) {
	testResults := []CheckResult{
		{
			Path:       "src/Service.php",
			Line:       10,
			Identifier: "phpstan/missingType",
			Message:    "Method has no return type",
			Severity:   SeverityError,
			Tip:        "Add a return type declaration",
		},
	}

	check := &testCheck{Results: testResults}

	output := captureOutput(func() {
		_ = doGitHubReport(check)
	})

	assert.Contains(t, output, "Method has no return type")
	assert.Contains(t, output, "%0A%0ATip: Add a return type declaration")
}

func TestGitLabReportWithTip(t *testing.T) {
	testResults := []CheckResult{
		{
			Path:       "src/Service.php",
			Line:       10,
			Identifier: "phpstan/missingType",
			Message:    "Method has no return type",
			Severity:   SeverityError,
			Tip:        "Add a return type declaration",
		},
	}

	check := &testCheck{Results: testResults}

	output := captureOutput(func() {
		err := doGitLabReport(check)
		assert.NoError(t, err)
	})

	var issues []GitLabCodeQualityIssue
	err := json.Unmarshal([]byte(output), &issues)
	assert.NoError(t, err)
	assert.Len(t, issues, 1)
	assert.Contains(t, issues[0].Description, "Method has no return type")
	assert.Contains(t, issues[0].Description, "Tip: Add a return type declaration")
}

func TestMarkdownReportWithTip(t *testing.T) {
	testResults := []CheckResult{
		{
			Path:       "src/Service.php",
			Line:       10,
			Identifier: "phpstan/missingType",
			Message:    "Method has no return type",
			Severity:   SeverityError,
			Tip:        "Add a return type declaration",
		},
	}

	check := &testCheck{Results: testResults}

	output := captureOutput(func() {
		_ = doMarkdownReport(check)
	})

	assert.Contains(t, output, "Method has no return type")
	assert.Contains(t, output, "*Tip: Add a return type declaration*")
}

func TestJSONReportWithTip(t *testing.T) {
	testResults := []CheckResult{
		{
			Path:       "src/Service.php",
			Line:       10,
			Identifier: "phpstan/missingType",
			Message:    "Method has no return type",
			Severity:   SeverityError,
			Tip:        "Add a return type declaration",
		},
	}

	check := &testCheck{Results: testResults}

	output := captureOutput(func() {
		err := doJSONReport(check)
		assert.NoError(t, err)
	})

	var result map[string][]CheckResult
	err := json.Unmarshal([]byte(output), &result)
	assert.NoError(t, err)
	assert.Len(t, result["results"], 1)
	assert.Equal(t, "Add a return type declaration", result["results"][0].Tip)
}

func TestJUnitReportWithTip(t *testing.T) {
	testResults := []CheckResult{
		{
			Path:       "src/Service.php",
			Line:       10,
			Identifier: "phpstan/missingType",
			Message:    "Method has no return type",
			Severity:   SeverityError,
			Tip:        "Add a return type declaration",
		},
		{
			Path:       "src/Other.php",
			Line:       5,
			Identifier: "phpstan/warning",
			Message:    "Some warning",
			Severity:   SeverityWarning,
			Tip:        "Consider fixing this",
		},
	}

	check := &testCheck{Results: testResults}

	output := captureOutput(func() {
		err := doJUnitReport(check)
		assert.NoError(t, err)
	})

	assert.Contains(t, output, "Method has no return type")
	assert.Contains(t, output, "Tip: Add a return type declaration")
	assert.Contains(t, output, "Some warning")
	assert.Contains(t, output, "Tip: Consider fixing this")
}

func TestJUnitReportPreservesLineInClassName(t *testing.T) {
	check := &testCheck{Results: []CheckResult{
		{
			Path:       "src/Service.php",
			Line:       24,
			Identifier: "phpstan/missingType",
			Message:    "Method has no return type",
			Severity:   SeverityError,
		},
	}}

	output := captureOutput(func() {
		err := doJUnitReport(check)
		assert.NoError(t, err)
	})

	assert.Contains(t, output, `classname="src/Service.php:24"`)
}

// captureOutput captures stdout during function execution
func captureOutput(fn func()) string {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	if err := w.Close(); err != nil {
		panic(err)
	}
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		panic(err)
	}
	return buf.String()
}

func TestSummaryLine(t *testing.T) {
	assert.Equal(t, "✓ No problems found", summaryLine(0, 0, 0))
	assert.Equal(t, "✖ 1 problem (1 error, 0 warnings)", summaryLine(1, 1, 0))
	assert.Equal(t, "✖ 15 problems (14 errors, 1 warning)", summaryLine(15, 14, 1))
}

func baselineCheck() *testCheck {
	return &testCheck{
		Target: &Target{Version: "6.6.10.21", Source: TargetSourceConstraint, Constraint: "~6.6.0 || ~6.7.0", WithinConstraint: true},
		ToolRuns: []ToolRun{
			{Name: "admin-twig", Status: ToolRunSkipped, Baseline: "6.6.10.21", Note: "no admin Twig rules apply to Shopware 6.6.10.21"},
			{Name: "phpstan", Status: ToolRunRan, Baseline: "6.7.14.2"},
			{Name: "rector", Status: ToolRunSkipped, Note: "no check operation"},
			{Name: "stylelint", Status: ToolRunRan},
		},
	}
}

func TestSummaryReportShowsBaselineAndNotEvaluatedTools(t *testing.T) {
	output := captureOutput(func() {
		_ = doSummaryReport(baselineCheck())
	})

	assert.True(t, strings.HasPrefix(output, "Shopware baseline: 6.6.10.21 (lowest release matching \"~6.6.0 || ~6.7.0\")\n"), output)
	assert.Contains(t, output, "\nNot evaluated against 6.6.10.21:\n  admin-twig       no admin Twig rules apply to Shopware 6.6.10.21\n  phpstan          evaluated Shopware 6.7.14.2\n")
	assert.NotContains(t, output, "rector")
	assert.NotContains(t, output, "stylelint")
	assert.True(t, strings.HasSuffix(output, "\n✓ No problems found\n"), output)
}

func TestSummaryReportWithoutBaselineIsUnchanged(t *testing.T) {
	output := captureOutput(func() {
		_ = doSummaryReport(&testCheck{})
	})

	assert.Equal(t, "\n✓ No problems found\n", output)
}

func TestMarkdownReportShowsBaselineAndNotEvaluatedTools(t *testing.T) {
	output := captureOutput(func() {
		_ = doMarkdownReport(baselineCheck())
	})

	assert.Contains(t, output, "# Validation Report\n\nShopware baseline: 6.6.10.21 (lowest release matching \"~6.6.0 || ~6.7.0\")\n\n")
	assert.Contains(t, output, "## Not evaluated against 6.6.10.21\n\n- **admin-twig**: no admin Twig rules apply to Shopware 6.6.10.21\n- **phpstan**: evaluated Shopware 6.7.14.2\n")
	assert.Contains(t, output, "✅ No problems found")
}

func TestGitHubReportEmitsBaselineNotice(t *testing.T) {
	output := captureOutput(func() {
		_ = doGitHubReport(baselineCheck())
	})

	assert.Contains(t, output, "Shopware baseline: 6.6.10.21 (lowest release matching \"~6.6.0 || ~6.7.0\")\n")
	assert.Contains(t, output, "::notice title=Shopware baseline::6.6.10.21 (lowest release matching \"~6.6.0 || ~6.7.0\")\n")
}

func TestJSONReportIncludesTargetAndChecks(t *testing.T) {
	output := captureOutput(func() {
		_ = doJSONReport(baselineCheck())
	})

	var data struct {
		Target  *Target       `json:"target"`
		Checks  []ToolRun     `json:"checks"`
		Results []CheckResult `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &data))
	assert.Equal(t, baselineCheck().Target, data.Target)
	assert.Equal(t, baselineCheck().ToolRuns, data.Checks)
}

func TestJSONReportWithoutBaselineHasEmptyChecks(t *testing.T) {
	output := captureOutput(func() {
		_ = doJSONReport(&testCheck{Results: []CheckResult{}})
	})

	assert.JSONEq(t, `{"checks": [], "results": []}`, output)
}

func TestGitLabReportKeepsStdoutMachineReadable(t *testing.T) {
	stderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	defer func() { os.Stderr = stderr }()

	output := captureOutput(func() {
		_ = doGitLabReport(baselineCheck())
	})

	require.NoError(t, w.Close())
	var errBuf bytes.Buffer
	_, err = io.Copy(&errBuf, r)
	require.NoError(t, err)

	var issues []GitLabCodeQualityIssue
	require.NoError(t, json.Unmarshal([]byte(output), &issues))
	assert.Empty(t, issues)
	assert.Contains(t, errBuf.String(), "Shopware baseline: 6.6.10.21")
}

func TestJUnitReportCarriesBaselineProperties(t *testing.T) {
	output := captureOutput(func() {
		_ = doJUnitReport(baselineCheck())
	})

	assert.Contains(t, output, `<property name="shopware.baseline" value="6.6.10.21"></property>`)
	assert.Contains(t, output, `<property name="shopware.baseline.source" value="constraint"></property>`)
}

func TestTargetDescribe(t *testing.T) {
	constraint := Target{Version: "6.6.0.0", Source: TargetSourceConstraint, Constraint: "~6.6.0", WithinConstraint: true}
	fallback := Target{Version: "6.7.0.0", Source: TargetSourceFallback, Constraint: ">=7.0"}

	assert.Equal(t, `6.6.0.0 (lowest release matching "~6.6.0")`, constraint.Describe())
	assert.Equal(t, `6.7.0.0 (fallback, no release matches ">=7.0")`, fallback.Describe())
}

func TestToolRunNotEvaluated(t *testing.T) {
	assert.False(t, ToolRun{Name: "stylelint", Status: ToolRunRan}.NotEvaluated("6.7.0.0"))
	assert.False(t, ToolRun{Name: "rector", Status: ToolRunSkipped}.NotEvaluated("6.7.0.0"))
	assert.False(t, ToolRun{Name: "eslint", Status: ToolRunRan, Baseline: "6.7.0.0"}.NotEvaluated("6.7.0.0"))
	assert.True(t, ToolRun{Name: "phpstan", Status: ToolRunRan, Baseline: "6.6.0.0"}.NotEvaluated("6.7.0.0"))
	assert.True(t, ToolRun{Name: "admin-twig", Status: ToolRunSkipped, Baseline: "6.7.0.0"}.NotEvaluated("6.7.0.0"))
}
