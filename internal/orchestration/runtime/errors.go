package runtime

import (
	"fmt"
	"strconv"
)

// ExitError reports a workload that finished with a non-zero exit code.
type ExitError struct {
	// Code is the workload's exit code.
	Code int
}

func (e *ExitError) Error() string {
	return "process exited with code " + strconv.Itoa(e.Code)
}

// ExecutionStage identifies where execution failed.
type ExecutionStage string

const (
	// StageStart identifies a failure to start a process.
	StageStart ExecutionStage = "start"

	// StageWait identifies a failure while observing a process.
	StageWait ExecutionStage = "wait"

	// StageStop identifies a failure to stop a process.
	StageStop ExecutionStage = "stop"

	// StageCleanup identifies a failure to clean up execution resources.
	StageCleanup ExecutionStage = "cleanup"
)

// ExecutionError reports a failure of the execution machinery.
type ExecutionError struct {
	// Stage identifies the operation that failed.
	Stage ExecutionStage

	// Err is the underlying cause.
	Err error
}

func (e *ExecutionError) Error() string {
	verb := string(e.Stage)

	switch e.Stage {
	case StageStart:
		verb = "start"
	case StageWait:
		verb = "observe"
	case StageStop:
		verb = "stop"
	case StageCleanup:
		verb = "clean up"
	}

	return fmt.Sprintf("cannot %s process: %v", verb, e.Err)
}

func (e *ExecutionError) Unwrap() error { return e.Err }
