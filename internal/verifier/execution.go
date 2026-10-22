package verifier

import (
	"bytes"
	"os/exec"
	"slices"
	"strconv"
	"sync"

	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/validation"
)

type ToolError = validation.ToolError

type SubprocessResult struct {
	Command []string
	Dir     string
	Stdout  string
	Stderr  string
	Err     error
}

func (r SubprocessResult) ToolError(err error) *ToolError {
	return &ToolError{Err: err, Command: slices.Clone(r.Command), Dir: r.Dir, Stdout: r.Stdout, Stderr: r.Stderr}
}

// ToolExecution delivers findings synchronously and keeps only the outcome.
// Callbacks must be safe for concurrent use; the runner serializes delivery.
type ToolExecution struct {
	mu           sync.Mutex
	result       ToolResult
	onFindings   func([]validation.CheckResult) bool
	onSubprocess func(SubprocessResult)
	telemetry    func(map[string]string)
}

func NewToolExecution(onFindings func([]validation.CheckResult) bool) *ToolExecution {
	return &ToolExecution{onFindings: onFindings}
}

// runDirectories waits for every directory, without cancelling sibling work
// when one invocation reports an error through ToolExecution.
func runDirectories(directories []string, run func(string)) {
	var group sync.WaitGroup
	for _, directory := range directories {
		group.Go(func() { run(directory) })
	}
	group.Wait()
}

func (e *ToolExecution) Run(cmd *exec.Cmd) SubprocessResult {
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	result := SubprocessResult{Command: slices.Clone(cmd.Args), Dir: cmd.Dir, Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
	if e.telemetry != nil {
		tags := map[string]string{
			tracking.TagStarted:     strconv.FormatBool(cmd.Process != nil),
			tracking.TagStdoutBytes: strconv.Itoa(stdout.Len()),
			tracking.TagStderrBytes: strconv.Itoa(stderr.Len()),
		}
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
			tags[tracking.TagExitCode] = strconv.Itoa(cmd.ProcessState.ExitCode())
		}
		e.telemetry(tags)
	}
	if e.onSubprocess != nil {
		e.onSubprocess(result)
	}
	return result
}

func (e *ToolExecution) AddFindings(findings ...validation.CheckResult) {
	if len(findings) == 0 {
		return
	}
	if e.onFindings(findings) {
		e.mu.Lock()
		e.result.HasFindings = true
		e.mu.Unlock()
	}
}

func (e *ToolExecution) Fail(err *ToolError) {
	if err == nil || err.Err == nil {
		panic("ToolExecution.Fail requires a non-nil error")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	failure := *err
	failure.Command = slices.Clone(err.Command)
	e.result.Errors = append(e.result.Errors, failure)
}

func (e *ToolExecution) Result() ToolResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := e.result
	result.Errors = slices.Clone(result.Errors)
	for i := range result.Errors {
		result.Errors[i].Command = slices.Clone(result.Errors[i].Command)
	}
	return result
}
