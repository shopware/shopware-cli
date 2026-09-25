package validation

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const reporterFormats = "summary, json, github, gitlab, junit, markdown"

// ValidateReporter checks whether name identifies a supported validation
// report format.
func ValidateReporter(name string) error {
	switch name {
	case "summary", "json", "github", "gitlab", "junit", "markdown":
		return nil
	default:
		return fmt.Errorf("invalid reporting format %q, allowed values: %s", name, reporterFormats)
	}
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

// ToolInvocationStatus records whether a tool was invoked or skipped.
type ToolInvocationStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// DoCheckReport reports findings and, when supplied, tool invocation statuses.
func DoCheckReport(result Check, reportingFormat string, tools ...ToolInvocationStatus) error {
	if err := ValidateReporter(reportingFormat); err != nil {
		return err
	}

	switch reportingFormat {
	case "summary":
		if err := doSummaryReport(result, tools...); err != nil {
			return err
		}
	case "json":
		if err := doJSONReport(result, tools...); err != nil {
			return err
		}
	case "github":
		if err := doGitHubReport(result, tools...); err != nil {
			return err
		}
	case "gitlab":
		if err := doGitLabReport(result); err != nil {
			return err
		}
		if err := PrintToolInvocationTable(os.Stderr, "Checkers", tools); err != nil {
			return err
		}
	case "markdown":
		if err := doMarkdownReport(result, tools...); err != nil {
			return err
		}
	case "junit":
		if err := doJUnitReport(result, tools...); err != nil {
			return err
		}
		if err := PrintToolInvocationTable(os.Stderr, "Checkers", tools); err != nil {
			return err
		}
	}

	if result.HasErrors() {
		return errors.New("found errors")
	}

	return nil
}

func doSummaryReport(result Check, tools ...ToolInvocationStatus) error {
	// Group results by file
	fileGroups := make(map[string][]CheckResult)
	for _, r := range result.GetResults() {
		fileGroups[r.Path] = append(fileGroups[r.Path], r)
	}

	// Sort results within each file group for deterministic output
	for path, results := range fileGroups {
		sort.Slice(results, func(i, j int) bool {
			// Sort by line number first, then by identifier, then by message
			if results[i].Line != results[j].Line {
				return results[i].Line < results[j].Line
			}
			if results[i].Identifier != results[j].Identifier {
				return results[i].Identifier < results[j].Identifier
			}
			return results[i].Message < results[j].Message
		})
		fileGroups[path] = results
	}

	// Get sorted list of file paths for deterministic output
	var sortedPaths []string
	for path := range fileGroups {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	// Print results grouped by file
	totalProblems := 0
	errorCount := 0
	warningCount := 0

	for _, file := range sortedPaths {
		results := fileGroups[file]
		//nolint:forbidigo
		fmt.Printf("\n%s\n", file)
		for _, r := range results {
			totalProblems++
			switch r.Severity {
			case SeverityError:
				errorCount++
			case SeverityWarning:
				warningCount++
			}
			//nolint:forbidigo
			fmt.Printf("  %d  %-7s  [%s]  %s\n", r.Line, r.Severity, r.Identifier, r.Message)
			if r.Tip != "" {
				//nolint:forbidigo
				fmt.Printf("             Tip: %s\n", r.Tip)
			}
		}
	}

	if err := PrintToolInvocationTable(os.Stdout, "Checkers", tools); err != nil {
		return err
	}

	//nolint:forbidigo
	if tools != nil && !anyToolInvoked(tools) {
		fmt.Println("\nNo checkers invoked; 0 problems reported")
	} else {
		fmt.Printf("\n%s\n", summaryLine(totalProblems, errorCount, warningCount))
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

func doJSONReport(result Check, tools ...ToolInvocationStatus) error {
	data := map[string]interface{}{
		"results": result.GetResults(),
	}
	if tools != nil {
		data["tools"] = tools
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func doGitHubReport(result Check, tools ...ToolInvocationStatus) error {
	// Print the human-readable summary first so the GitHub Actions log
	// shows file/line context, then emit annotations for PR inline display.
	// File paths and messages can contain `::` which the runner would parse
	// as a workflow command, so wrap the summary in stop-commands/resume
	// using a random token to neutralize any embedded commands.
	var tokenBytes [16]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return err
	}
	token := hex.EncodeToString(tokenBytes[:])

	fmt.Printf("::stop-commands::%s\n", token)
	if err := doSummaryReport(result, tools...); err != nil {
		fmt.Printf("::%s::\n", token)
		return err
	}
	fmt.Printf("::%s::\n", token)

	// Sort results for deterministic output
	results := result.GetResults()
	sort.Slice(results, func(i, j int) bool {
		// Sort by path first, then by line number, then by identifier, then by message
		if results[i].Path != results[j].Path {
			return results[i].Path < results[j].Path
		}
		if results[i].Line != results[j].Line {
			return results[i].Line < results[j].Line
		}
		if results[i].Identifier != results[j].Identifier {
			return results[i].Identifier < results[j].Identifier
		}
		return results[i].Message < results[j].Message
	})

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

		fmt.Printf("::%s file=%s%s,title=%s::%s\n", level, file, line, r.Identifier, message)
	}

	return nil
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

func doGitLabReport(result Check) error {
	issues := make([]GitLabCodeQualityIssue, 0)

	// Sort results for deterministic output
	results := result.GetResults()
	sort.Slice(results, func(i, j int) bool {
		// Sort by path first, then by line number, then by identifier, then by message
		if results[i].Path != results[j].Path {
			return results[i].Path < results[j].Path
		}
		if results[i].Line != results[j].Line {
			return results[i].Line < results[j].Line
		}
		if results[i].Identifier != results[j].Identifier {
			return results[i].Identifier < results[j].Identifier
		}
		return results[i].Message < results[j].Message
	})

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
	return encoder.Encode(issues)
}

func doMarkdownReport(result Check, tools ...ToolInvocationStatus) error {
	// Group results by file
	fileGroups := make(map[string][]CheckResult)
	for _, r := range result.GetResults() {
		if r.Path == "" {
			r.Path = "general"
		}

		fileGroups[r.Path] = append(fileGroups[r.Path], r)
	}

	// Sort results within each file group for deterministic output
	for path, results := range fileGroups {
		sort.Slice(results, func(i, j int) bool {
			// Sort by line number first, then by identifier, then by message
			if results[i].Line != results[j].Line {
				return results[i].Line < results[j].Line
			}
			if results[i].Identifier != results[j].Identifier {
				return results[i].Identifier < results[j].Identifier
			}
			return results[i].Message < results[j].Message
		})
		fileGroups[path] = results
	}

	// Get sorted list of file paths for deterministic output
	var sortedPaths []string
	for path := range fileGroups {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	fmt.Println("# Validation Report")
	fmt.Println()

	totalProblems := 0
	for _, path := range sortedPaths {
		results := fileGroups[path]
		totalProblems += len(results)
		if len(results) == 0 {
			continue
		}

		fmt.Printf("## %s (%d problems)\n\n", path, len(results))
		for _, r := range results {
			severity := "⚠️ Warning"
			if r.Severity == SeverityError {
				severity = "❌ Error"
			}

			location := ""
			if r.Line > 0 {
				location = fmt.Sprintf(":%d", r.Line)
			}

			fmt.Printf("- **%s** %s%s: %s (`%s`)\n", severity, r.Path, location, r.Message, r.Identifier)
			if r.Tip != "" {
				fmt.Printf("  - *Tip: %s*\n", r.Tip)
			}
		}
		fmt.Println()
	}

	if tools != nil {
		fmt.Println("## Checkers")
		fmt.Println()
		fmt.Println("```text")
		for _, row := range toolInvocationRows(tools) {
			fmt.Println(row)
		}
		fmt.Println("```")
		fmt.Println()
	}

	if tools != nil && !anyToolInvoked(tools) {
		fmt.Println("No checkers invoked; 0 problems reported")
	} else if totalProblems == 0 {
		fmt.Println("✅ No problems found")
	}

	return nil
}

// PrintToolInvocationTable writes an aligned table of invoked and skipped tools.
func PrintToolInvocationTable(w io.Writer, title string, tools []ToolInvocationStatus) error {
	if tools == nil {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\n"+title+":"); err != nil {
		return err
	}
	for _, row := range toolInvocationRows(tools) {
		if _, err := fmt.Fprintln(w, "  "+row); err != nil {
			return err
		}
	}
	return nil
}

func toolInvocationRows(tools []ToolInvocationStatus) []string {
	width := 0
	for _, tool := range tools {
		width = max(width, len(tool.Name))
	}

	rows := make([]string, 0, len(tools))
	for _, tool := range tools {
		row := fmt.Sprintf("%-*s  %-7s", width, tool.Name, tool.Status)
		if tool.Reason != "" {
			row += "  " + tool.Reason
		}
		rows = append(rows, strings.TrimRight(row, " "))
	}
	return rows
}

func anyToolInvoked(tools []ToolInvocationStatus) bool {
	for _, tool := range tools {
		if tool.Status == "invoked" {
			return true
		}
	}
	return false
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

func doJUnitReport(result Check, tools ...ToolInvocationStatus) error {
	var testCases []JUnitTestCase
	errors := 0
	failures := 0
	skipped := 0

	// Sort results for deterministic output
	results := result.GetResults()
	sort.Slice(results, func(i, j int) bool {
		// Sort by path first, then by line number, then by identifier, then by message
		if results[i].Path != results[j].Path {
			return results[i].Path < results[j].Path
		}
		if results[i].Line != results[j].Line {
			return results[i].Line < results[j].Line
		}
		if results[i].Identifier != results[j].Identifier {
			return results[i].Identifier < results[j].Identifier
		}
		return results[i].Message < results[j].Message
	})

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
		if tool.Status == "skipped" {
			testCase.Skipped = &JUnitTestSkipped{Message: tool.Reason}
			skipped++
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
