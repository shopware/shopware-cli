package validation

import (
	"encoding/json"

	"github.com/shopware/shopware-cli/logging"
)

// ErrToolReportFailed matches every ToolReportError with errors.Is.
var ErrToolReportFailed = logging.NewSilent("tool report contains errors")

// ToolReportError fails a command whose tool report already shows the problems and names the tools behind them.
type ToolReportError struct {
	// FailedTools reported execution errors, so the report shows them as failed.
	FailedTools []string
	// ToolsWithErrorFindings ran and reported findings with error severity.
	ToolsWithErrorFindings []string
}

func (e *ToolReportError) Error() string {
	return ErrToolReportFailed.Error()
}

// SilenceError prevents logging the tool report a second time.
func (e *ToolReportError) SilenceError() {}

// Is lets errors.Is match the error against ErrToolReportFailed.
func (e *ToolReportError) Is(target error) bool {
	return target == ErrToolReportFailed
}

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
