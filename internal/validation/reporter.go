package validation

import (
	"cmp"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

// ReporterFormats lists the supported validation report formats.
var ReporterFormats = []string{"summary", "json", "github", "gitlab", "junit", "markdown"}

// ValidateReporter checks whether name identifies a supported validation
// report format.
func ValidateReporter(name string) error {
	if slices.Contains(ReporterFormats, strings.ToLower(name)) {
		return nil
	}

	return fmt.Errorf("invalid reporting format %q, allowed values: %s", name, strings.Join(ReporterFormats, ", "))
}

func DetectDefaultReporter() string {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return "github"
	}

	if os.Getenv("GITLAB_CI") == "true" {
		return "gitlab"
	}

	return "summary"
}

// toolInvocationStatus records whether a tool was invoked or skipped.
type toolInvocationStatus struct {
	Name               string      `json:"name"`
	Status             string      `json:"status"`
	Reason             string      `json:"reason,omitempty"`
	Note               string      `json:"note,omitempty"`
	HasFindings        bool        `json:"has_findings"`
	Errors             []ToolError `json:"errors,omitempty"`
	FindingsIncomplete bool        `json:"findings_incomplete,omitempty"`
}

// ReportTools renders collected findings and outcomes, preserving existing
// report formats and adding execution diagnostics where supported.
func ReportTools(tools []ToolReport, reportingFormat, title string) error {
	reportingFormat = strings.ToLower(reportingFormat)
	if err := ValidateReporter(reportingFormat); err != nil {
		return err
	}
	tools = slices.Clone(tools)
	slices.SortFunc(tools, func(a, b ToolReport) int { return cmp.Compare(a.Name, b.Name) })
	findings := make([]CheckResult, 0)
	statuses := make([]toolInvocationStatus, 0, len(tools))
	failed := false
	for _, tool := range tools {
		findings = append(findings, tool.Findings...)
		status := toolInvocationStatus{Name: tool.Name, Status: "invoked", Note: tool.Note, HasFindings: tool.HasFindings}
		if tool.Status() == "skipped" {
			status.Status, status.Reason = "skipped", tool.SkipReason
		}
		if len(tool.Errors) > 0 {
			failed = true
			status.Errors = tool.Errors
			status.FindingsIncomplete = tool.HasFindings
		}
		for _, finding := range tool.Findings {
			if finding.Severity == SeverityError {
				failed = true
			}
		}
		statuses = append(statuses, status)
	}
	var err error
	switch reportingFormat {
	case "summary":
		err = writeToolSummary(os.Stdout, tools, title, false)
	case "markdown":
		err = writeToolSummary(os.Stdout, tools, title, true)
	case "json":
		err = doJSONReport(findings, statuses...)
	case "github":
		err = doGitHubReport(findings, tools, title)
	case "gitlab":
		err = doGitLabReport(findings, tools, title)
	case "junit":
		err = doJUnitReport(findings, tools...)
		if err == nil {
			err = writeToolSummary(os.Stderr, tools, title, false)
		}
	}
	if err != nil {
		return err
	}
	if failed {
		return ErrToolReportFailed
	}
	return nil
}

// summaryLine renders the closing line of the summary report.
func summaryLine(total, errorCount, warningCount int) string {
	if total == 0 {
		return "✓ No problems found"
	}

	return fmt.Sprintf("✖ %s (%s, %s)", countNoun(total, "problem"), countNoun(errorCount, "error"), countNoun(warningCount, "warning"))
}

func countNoun(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}

	return fmt.Sprintf("%d %ss", n, noun)
}

func doJSONReport(findings []CheckResult, tools ...toolInvocationStatus) error {
	data := map[string]interface{}{
		"results": findings,
	}
	if tools != nil {
		data["tools"] = tools
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func doGitHubReport(findings []CheckResult, tools []ToolReport, title string) error {
	// Print the human-readable summary first so the GitHub Actions log
	// shows file/line context, then emit annotations for PR inline display.
	// File paths and messages can contain `::` which the runner would parse
	// as a workflow command, so wrap the summary in stop-commands/resume
	// using a random token to neutralize any embedded commands.
	var output strings.Builder
	var tokenBytes [16]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes[:])

	fmt.Fprintf(&output, "::stop-commands::%s\n", token)
	if err := writeToolSummary(&output, tools, title, false); err != nil {
		fmt.Fprintf(&output, "::%s::\n", token)
		return err
	}
	fmt.Fprintf(&output, "::%s::\n", token)

	// Sort results for deterministic output
	results := sortedFindings(findings)

	for _, r := range results {
		file := r.Path
		if file == "" {
			file = "."
		}

		level := SeverityWarning
		if r.Severity == SeverityError {
			level = SeverityError
		}

		line := ""
		if r.Line > 0 {
			line = fmt.Sprintf(",line=%d", r.Line)
		}

		message := strings.ReplaceAll(r.Message, "\n", "%0A")
		message = strings.ReplaceAll(message, "\r", "%0D")
		if r.Tip != "" {
			tip := strings.ReplaceAll(r.Tip, "\n", "%0A")
			tip = strings.ReplaceAll(tip, "\r", "%0D")
			message = message + "%0A%0ATip: " + tip
		}

		fmt.Fprintf(&output, "::%s file=%s%s,title=%s::%s\n", level, file, line, r.Identifier, message)
	}

	for _, tool := range tools {
		for _, failure := range tool.Errors {
			message := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(failure.Error())
			fmt.Fprintf(&output, "::error title=%s::%s\n", tool.Name, message)
		}
	}
	_, err := io.WriteString(os.Stdout, output.String())
	return err
}

type GitLabCodeQualityIssue struct {
	Description string                    `json:"description"`
	CheckName   string                    `json:"check_name"`
	Fingerprint string                    `json:"fingerprint"`
	Severity    string                    `json:"severity"`
	Location    GitLabCodeQualityLocation `json:"location"`
}

type GitLabCodeQualityLocation struct {
	Path  string                 `json:"path"`
	Lines GitLabCodeQualityLines `json:"lines"`
}

type GitLabCodeQualityLines struct {
	Begin int `json:"begin"`
}

func doGitLabReport(findings []CheckResult, tools []ToolReport, title string) error {
	for _, tool := range tools {
		for _, failure := range tool.Errors {
			findings = append(findings, CheckResult{Path: ".", Line: 1, Severity: SeverityError, Identifier: tool.Name + "/execution", Message: toolErrorDetail(tool, failure)})
		}
	}

	issues := make([]GitLabCodeQualityIssue, 0)

	// Sort results for deterministic output
	results := sortedFindings(findings)

	for _, r := range results {
		// Convert severity to GitLab format
		severity := "minor"
		switch r.Severity {
		case SeverityError:
			severity = "major"
		case SeverityWarning:
			severity = "minor"
		}

		// Create fingerprint (unique identifier for the issue)
		fingerprintData := fmt.Sprintf("%s:%d:%s:%s", r.Path, r.Line, r.Identifier, r.Message)
		fingerprint := fmt.Sprintf("%x", md5.Sum([]byte(fingerprintData)))

		description := r.Message
		if r.Tip != "" {
			description = description + "\n\nTip: " + r.Tip
		}

		issue := GitLabCodeQualityIssue{
			Description: description,
			CheckName:   r.Identifier,
			Fingerprint: fingerprint,
			Severity:    severity,
			Location: GitLabCodeQualityLocation{
				Path: r.Path,
				Lines: GitLabCodeQualityLines{
					Begin: r.Line,
				},
			},
		}

		issues = append(issues, issue)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(issues); err != nil {
		return err
	}
	return writeToolSummary(os.Stderr, tools, title, false)
}

type JUnitTestSuite struct {
	XMLName  xml.Name        `xml:"testsuite"`
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Errors   int             `xml:"errors,attr"`
	Skipped  int             `xml:"skipped,attr,omitempty"`
	TestCase []JUnitTestCase `xml:"testcase"`
}

type JUnitTestCase struct {
	Name      string            `xml:"name,attr"`
	ClassName string            `xml:"classname,attr"`
	Failure   *JUnitTestFailure `xml:"failure,omitempty"`
	Error     *JUnitTestError   `xml:"error,omitempty"`
	Skipped   *JUnitTestSkipped `xml:"skipped,omitempty"`
}

type JUnitTestSkipped struct {
	Message string `xml:"message,attr"`
}

type JUnitTestFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

type JUnitTestError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

func doJUnitReport(findings []CheckResult, tools ...ToolReport) error {
	var testCases []JUnitTestCase
	errors := 0
	failures := 0
	skipped := 0

	// Sort results for deterministic output
	results := sortedFindings(findings)

	for _, r := range results {
		className := r.Path
		if r.Line > 0 {
			className = fmt.Sprintf("%s:%d", r.Path, r.Line)
		}

		testCase := JUnitTestCase{
			Name:      r.Identifier,
			ClassName: className,
		}

		content := r.Message
		if r.Tip != "" {
			content = content + "\n\nTip: " + r.Tip
		}

		if r.Severity == SeverityError {
			testCase.Error = &JUnitTestError{
				Message: r.Message,
				Type:    r.Identifier,
				Content: content,
			}
			errors++
		} else {
			testCase.Failure = &JUnitTestFailure{
				Message: r.Message,
				Type:    r.Identifier,
				Content: content,
			}
			failures++
		}

		testCases = append(testCases, testCase)
	}
	for _, tool := range tools {
		testCase := JUnitTestCase{Name: tool.Name, ClassName: "tool"}
		if tool.Status() == "skipped" {
			testCase.Skipped = &JUnitTestSkipped{Message: tool.SkipReason}
			skipped++
		}

		if len(tool.Errors) > 0 {
			for _, failure := range tool.Errors {
				testCases = append(testCases, JUnitTestCase{Name: tool.Name, ClassName: "tool", Error: &JUnitTestError{Message: failure.Error(), Type: "tool-execution", Content: toolErrorDetail(tool, failure)}})
				errors++
			}
			continue
		}

		testCases = append(testCases, testCase)
	}

	suite := JUnitTestSuite{
		Name:     "shopware-cli-validation",
		Tests:    len(testCases),
		Failures: failures,
		Errors:   errors,
		Skipped:  skipped,
		TestCase: testCases,
	}

	encoder := xml.NewEncoder(os.Stdout)
	encoder.Indent("", "  ")
	return encoder.Encode(suite)
}

func sortedFindings(findings []CheckResult) []CheckResult {
	findings = slices.Clone(findings)
	slices.SortFunc(findings, func(a, b CheckResult) int {
		return cmp.Or(
			cmp.Compare(a.Path, b.Path),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Identifier, b.Identifier),
			cmp.Compare(a.Message, b.Message),
		)
	})
	return findings
}
