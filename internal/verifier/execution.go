package verifier

import (
	"bytes"
	"os"
	"os/exec"
	"slices"
	"sync"
	"time"

	"github.com/shopware/shopware-cli/internal/validation"
)

type ToolError = validation.ToolError

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

// ToolExecution delivers findings synchronously and keeps execution failures.
// The callback must be safe for concurrent use.
type ToolExecution struct {
	mu           sync.Mutex
	result       ToolResult
	onFindings   func([]validation.CheckResult) bool
	onSubprocess func(SubprocessResult)
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
	e.result.Errors = append(e.result.Errors, *err)
}

func (e *ToolExecution) snapshot() ToolResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.result
}
