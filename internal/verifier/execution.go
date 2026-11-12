package verifier

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/shopware/shopware-cli/internal/validation"
)

type ToolError = validation.ToolError

// Subprocess operations identify tool attempts in telemetry.
const (
	OperationCheck          = "check"
	OperationFix            = "fix"
	OperationFormat         = "format"
	OperationAnalyse        = "analyse"
	OperationComposerUpdate = "composer_update"
)

type SubprocessResult struct {
	Operation string
	Command   []string
	Dir       string
	Stdout    string
	Stderr    string
	Err       error
	// Duration measures the command attempt, including time spent starting it.
	Duration time.Duration
	// ProcessState is nil when the process could not start.
	ProcessState *os.ProcessState
}

func (r SubprocessResult) ToolError(err error) *ToolError {
	return &ToolError{Err: err, Command: r.Command, Dir: r.Dir, Stdout: r.Stdout, Stderr: r.Stderr}
}

// cpuTimeCoversChildren is false on Windows, where a process's CPU time leaves out the processes it starts,
// so cpu_ms is not sent there. It is a variable so tests can cover both cases.
var cpuTimeCoversChildren = runtime.GOOS != "windows"

// cpuTime is the user and system CPU time of the process and the children it waited for.
func (r SubprocessResult) cpuTime() time.Duration {
	if r.ProcessState == nil {
		return 0
	}
	return r.ProcessState.UserTime() + r.ProcessState.SystemTime()
}

// ToolExecution delivers findings synchronously and collects applied changes and failures.
// Finding callbacks run under the mutex and must not call back into the helper.
type ToolExecution struct {
	mu           sync.Mutex
	finalized    bool
	result       ToolResult
	onFindings   func([]validation.CheckResult) bool
	onSubprocess func(SubprocessResult)
}

func NewToolExecution(onFindings func([]validation.CheckResult) bool) *ToolExecution {
	return &ToolExecution{onFindings: onFindings}
}

// runDirectories waits for every directory, without cancelling sibling work
// when one invocation reports an error through ToolExecution.
func runDirectories(ctx context.Context, directories []string, run func(string)) {
	var group sync.WaitGroup
	for _, directory := range directories {
		if ctx.Err() != nil {
			break
		}
		group.Go(func() {
			if ctx.Err() == nil {
				run(directory)
			}
		})
	}
	group.Wait()
}

// Run executes cmd and records its output and telemetry. Prefer one of the
// Operation constants for operation to keep telemetry values consistent.
func (e *ToolExecution) Run(operation string, cmd *exec.Cmd) SubprocessResult {
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	started := time.Now()
	err := cmd.Run()
	duration := time.Since(started)
	result := SubprocessResult{
		Operation:    operation,
		Command:      slices.Clone(cmd.Args),
		Dir:          cmd.Dir,
		Stdout:       stdout.String(),
		Stderr:       stderr.String(),
		Err:          err,
		Duration:     duration,
		ProcessState: cmd.ProcessState,
	}
	if e.onSubprocess != nil {
		e.onSubprocess(result)
	}
	return result
}

func (e *ToolExecution) AddFindings(findings ...validation.CheckResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.assertOpen()
	if len(findings) == 0 {
		return
	}
	if e.onFindings(findings) {
		e.result.HasFindings = true
	}
}

// AddChangedFiles records confirmed applied changes, including partial successes.
// Paths must be absolute or relative to ToolConfig.RootDir. RunTools normalizes
// and deduplicates them after the tool and its cleanup finish.
func (e *ToolExecution) AddChangedFiles(paths ...string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.assertOpen()
	e.result.ChangedFiles = append(e.result.ChangedFiles, paths...)
}

// parsedToolReport retains valid entries alongside malformed report errors.
// Changed paths describe native output fields, not remaining findings.
type parsedToolReport struct {
	findings     []validation.CheckResult
	changedFiles []string
	errors       []error
}

func (e *ToolExecution) Fail(err *ToolError) {
	if err == nil || err.Err == nil {
		panic("ToolExecution.Fail requires a non-nil error")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.assertOpen()
	e.result.Errors = append(e.result.Errors, *err)
}

// finalize transfers the result to the runner after tool cleanup and records
// cancellation unless it was already reported. Further reporting is a programmer error.
func (e *ToolExecution) finalize(cancellation error) ToolResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.assertOpen()
	if cancellation != nil && !slices.ContainsFunc(e.result.Errors, func(failure ToolError) bool {
		return errors.Is(failure, cancellation)
	}) {
		e.result.Errors = append(e.result.Errors, ToolError{Err: cancellation})
	}
	e.finalized = true
	return e.result
}

// assertOpen requires mu to be held.
func (e *ToolExecution) assertOpen() {
	if e.finalized {
		panic("ToolExecution is finalized")
	}
}
