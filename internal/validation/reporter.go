package validation

import (
	"crypto/md5"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strings"
)

func DetectDefaultReporter() string {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return "github"
	}

	if os.Getenv("GITLAB_CI") == "true" {
		return "gitlab"
	}

	return "summary"
}

func DoCheckReport(result Check, reportingFormat string) error {
	switch reportingFormat {
	case "summary":
		if err := doSummaryReport(result); err != nil {
			return err
		}
	case "json":
		if err := doJSONReport(result); err != nil {
			return err
		}
	case "github":
		if err := doGitHubReport(result); err != nil {
			return err
		}
	case "gitlab":
		if err := doGitLabReport(result); err != nil {
			return err
		}
	case "markdown":
		if err := doMarkdownReport(result); err != nil {
			return err
		}
	case "junit":
		if err := doJUnitReport(result); err != nil {
			return err
		}
	}

	if result.HasErrors() {
		return fmt.Errorf("found errors")
	}

	return nil
}

func doSummaryReport(result Check) error {
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
			fmt.Printf("  %d  %-7s  %s  %s\n", r.Line, r.Severity, r.Message, r.Identifier)
			if r.Tip != "" {
				//nolint:forbidigo
				fmt.Printf("             Tip: %s\n", r.Tip)
			}
		}
	}

	//nolint:forbidigo
	fmt.Printf("\n✖ %d problems (%d errors, %d warnings)\n", totalProblems, errorCount, warningCount)

	return nil
}

func doJSONReport(result Check) error {
	data := map[string]interface{}{
		"results": result.GetResults(),
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func doGitHubReport(result Check) error {
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

func doMarkdownReport(result Check) error {
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

	if totalProblems == 0 {
		fmt.Println("✅ No problems found")
	}

	return nil
}

type JUnitTestSuite struct {
	XMLName  xml.Name        `xml:"testsuite"`
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Errors   int             `xml:"errors,attr"`
	TestCase []JUnitTestCase `xml:"testcase"`
}

type JUnitTestCase struct {
	Name      string            `xml:"name,attr"`
	ClassName string            `xml:"classname,attr"`
	Failure   *JUnitTestFailure `xml:"failure,omitempty"`
	Error     *JUnitTestError   `xml:"error,omitempty"`
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

func doJUnitReport(result Check) error {
	var testCases []JUnitTestCase
	errors := 0
	failures := 0

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
		testCase := JUnitTestCase{
			Name:      r.Identifier,
			ClassName: r.Path,
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

	suite := JUnitTestSuite{
		Name:     "shopware-cli-validation",
		Tests:    len(testCases),
		Failures: failures,
		Errors:   errors,
		TestCase: testCases,
	}

	encoder := xml.NewEncoder(os.Stdout)
	encoder.Indent("", "  ")
	return encoder.Encode(suite)
}
