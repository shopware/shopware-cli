package tracking

// FailedStepError names the step a command failed at, which the command event reports as failed_step.
type FailedStepError struct {
	Step string
	Err  error
}

// FailedAt wraps err with the step it occurred at; a nil err stays nil.
func FailedAt(step string, err error) error {
	if err == nil {
		return nil
	}
	return &FailedStepError{Step: step, Err: err}
}

func (e *FailedStepError) Error() string { return e.Err.Error() }
func (e *FailedStepError) Unwrap() error { return e.Err }
