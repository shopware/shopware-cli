package runtime

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnsupported reports a capability or setting the runtime cannot provide.
var ErrUnsupported = errors.New("unsupported runtime capability")

// ExitError reports an unsuccessful workload exit. The executor returns it only
// when the runtime reported no failure of its own, so an error is either an
// ExitError or a runtime failure. A consumer applies its own exit-code policy.
type ExitError struct {
	// Code is the workload exit code, or -1 when terminated without an exit code.
	Code int
	// Signal is the name of the signal that ended the workload, for example
	// SIGKILL. It is empty when the workload was not ended by a signal or when
	// that is unknown.
	Signal string
}

// Error describes the workload's unsuccessful exit by its signal or exit code.
func (e *ExitError) Error() string {
	if e.Signal != "" {
		return fmt.Sprintf("process was terminated by signal %q", e.Signal)
	}

	if e.Code < 0 {
		return "process was terminated without an exit code"
	}

	return fmt.Sprintf("process exited with code %d", e.Code)
}

// JoinErrors joins independent failures like errors.Join, but on one line,
// because tool reports embed the message. It returns nil without failures.
func JoinErrors(errs ...error) error {
	joined := &joinedError{}
	for _, err := range errs {
		if err != nil {
			joined.errs = append(joined.errs, err)
		}
	}

	if len(joined.errs) == 0 {
		return nil
	}

	return joined
}

type joinedError struct {
	errs []error
}

func (e *joinedError) Error() string {
	messages := make([]string, len(e.errs))
	for i, err := range e.errs {
		messages[i] = err.Error()
	}

	return strings.Join(messages, "; ")
}

func (e *joinedError) Unwrap() []error { return e.errs }
