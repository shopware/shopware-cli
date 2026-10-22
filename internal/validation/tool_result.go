package validation

import "errors"

// ErrToolReportFailed indicates a failure already described by a tool report.
var ErrToolReportFailed = errors.New("tool report contains errors")

// ToolResult describes one tool's outcome. A skipped tool has neither findings
// nor an execution error. Findings and captured output survive execution errors.
type ToolResult struct {
	Name       string
	SkipReason string
	Findings   []CheckResult
	Note       string
	Stdout     string
	Stderr     string
	Err        error
}

// Status describes execution, independently of finding severities.
func (r ToolResult) Status() string {
	if r.SkipReason != "" {
		return "skipped"
	}
	if r.Err != nil {
		return "failed"
	}
	return "completed"
}
