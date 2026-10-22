package validation

import (
	"encoding/json"
	"errors"
)

// ErrToolReportFailed indicates a failure already described by a tool report.
var ErrToolReportFailed = errors.New("tool report contains errors")

// ToolResult describes one invocation, independently of its collected findings.
type ToolResult struct {
	Name        string
	SkipReason  string
	HasFindings bool
	Note        string
	Errors      []ToolError
}

// ToolError describes a failure and, when available, its subprocess context.
type ToolError struct {
	Err     error    `json:"-"`
	Command []string `json:"command,omitempty"`
	Dir     string   `json:"dir,omitempty"`
	Stdout  string   `json:"stdout,omitempty"`
	Stderr  string   `json:"stderr,omitempty"`
}

func (e ToolError) Error() string { return e.Err.Error() }
func (e ToolError) Unwrap() error { return e.Err }

func (e ToolError) MarshalJSON() ([]byte, error) {
	type fields ToolError
	return json.Marshal(struct {
		Message string `json:"message"`
		fields
	}{Message: e.Error(), fields: fields(e)})
}

// ToolReport combines a summary with findings collected by the runner.
type ToolReport struct {
	ToolResult
	Findings []CheckResult
}

// Status describes execution, independently of finding severities.
func (r ToolResult) Status() string {
	if len(r.Errors) > 0 {
		return "failed"
	}
	if r.SkipReason != "" {
		return "skipped"
	}
	return "completed"
}
